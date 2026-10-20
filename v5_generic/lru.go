package v5_generic

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/ValeriyTyutyunnik/go-lru-evolution/buckets"
)

type HashableKey interface {
	comparable
	~string | ~int | ~uint32 | ~uint64
}

type listNode[K HashableKey] struct {
	key  K
	val  interface{}
	prev *listNode[K]
	next *listNode[K]
}

type cacheLRU[K HashableKey] struct {
	cap  int
	mu   sync.RWMutex
	hash map[K]*listNode[K]
	head *listNode[K]
	tail *listNode[K]
}

type Cache[K HashableKey] struct {
	shardCapacity int
	shardCount    int
	shards        []*cacheLRU[K]
}

func NewCache[K HashableKey](shardCapacity int, shardCount int) (*Cache[K], error) {
	if shardCount == 0 || (shardCount&(shardCount-1)) != 0 {
		return nil, errors.New("значение shardCount должно быть степенью двойки")
	}
	if shardCapacity < 2 {
		slog.Warn("Некорректное значение capacity. инициализация кэша с shardCapacity=2")
		shardCapacity = 2
	}
	res := &Cache[K]{shardCapacity: shardCapacity, shardCount: shardCount}
	res.shards = make([]*cacheLRU[K], shardCount)
	for i := range shardCount {
		res.shards[i] = &cacheLRU[K]{cap: shardCapacity, hash: make(map[K]*listNode[K], shardCapacity)}
	}

	return res, nil
}

func (c *Cache[K]) String() string {
	r := fmt.Sprintf("Cache shards=%v, shardCapacity=%d, totalCapacity=%d", c.shardCount, c.shardCapacity, int(c.shardCount)*c.shardCapacity)
	return r
}

func (c *Cache[K]) getShardNo[T HashableKey](key T) int {
	switch v := any(key).(type) {
	case string:
		return int(buckets.ForStr(v, uint64(c.shardCount)))
	case uint32:
		return int(buckets.For64(uint64(v), uint64(c.shardCount)))
	case uint64:
		return int(buckets.For64(v, uint64(c.shardCount)))
	case int:
		return int(buckets.For64(uint64(v), uint64(c.shardCount)))
	default:
		return 0
	}
}

func (c *cacheLRU[K]) moveToHead(list *listNode[K]) {
	if c.head == list {
		return
	}

	if c.tail == list {
		c.tail = list.prev
	}

	if list.prev != nil {
		list.prev.next = list.next
	}

	if list.next != nil {
		list.next.prev = list.prev
	}

	list.prev = nil
	list.next = c.head

	if c.head != nil {
		c.head.prev = list
	}
	c.head = list

	if c.tail == nil {
		c.tail = list
	}
}

func (c *cacheLRU[K]) deleteNode(list *listNode[K]) {
	if list != nil {
		delete(c.hash, list.key)
		if list.prev != nil {
			list.prev.next = list.next
		}
		if list.next != nil {
			list.next.prev = list.prev
		}
		if c.tail == list {
			c.tail = list.prev
		}
		if c.head == list {
			c.head = list.next
		}
		list.prev = nil
		list.next = nil
		list.val = ""
	}
}

func (c *cacheLRU[K]) deleteNodeByKey(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if list, ok := c.hash[key]; ok {
		c.deleteNode((list))
	}
}

func (c *cacheLRU[K]) get(key K) interface{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	list, ok := c.hash[key]
	if !ok {
		return ""
	}

	c.moveToHead(list)

	return list.val
}

func (c *cacheLRU[K]) put(key K, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if list, ok := c.hash[key]; ok {
		list.val = value
		c.moveToHead(list)
		return
	}

	if len(c.hash) >= c.cap {
		c.deleteNode(c.tail)
	}

	list := &listNode[K]{key: key, val: value}
	c.hash[key] = list
	c.moveToHead(list)
}

func (c *Cache[K]) Get(key K) interface{} {
	return c.shards[c.getShardNo(key)].get(key)
}

func (c *Cache[K]) Put(key K, value interface{}) {
	c.shards[c.getShardNo(key)].put(key, value)
}

func (c *Cache[K]) DeleteNodeByKey(key K) {
	c.shards[c.getShardNo(key)].deleteNodeByKey(key)
}
