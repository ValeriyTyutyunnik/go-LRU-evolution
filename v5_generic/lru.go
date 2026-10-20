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

type listNode[K HashableKey, V any] struct {
	key  K
	val  V
	prev *listNode[K, V]
	next *listNode[K, V]
}

type cacheLRU[K HashableKey, V any] struct {
	cap  int
	mu   sync.RWMutex
	hash map[K]*listNode[K, V]
	head *listNode[K, V]
	tail *listNode[K, V]
}

type Cache[K HashableKey, V any] struct {
	shardCapacity int
	shardCount    int
	shards        []*cacheLRU[K, V]
}

func NewCache[K HashableKey, V any](shardCapacity int, shardCount int) (*Cache[K, V], error) {
	if shardCount == 0 || (shardCount&(shardCount-1)) != 0 {
		return nil, errors.New("значение shardCount должно быть степенью двойки")
	}
	if shardCapacity < 2 {
		slog.Warn("Некорректное значение capacity. инициализация кэша с shardCapacity=2")
		shardCapacity = 2
	}
	res := &Cache[K, V]{shardCapacity: shardCapacity, shardCount: shardCount}
	res.shards = make([]*cacheLRU[K, V], shardCount)
	for i := range shardCount {
		res.shards[i] = &cacheLRU[K, V]{cap: shardCapacity, hash: make(map[K]*listNode[K, V], shardCapacity)}
	}

	return res, nil
}

func (c *Cache[K, V]) String() string {
	r := fmt.Sprintf("Cache shards=%v, shardCapacity=%d, totalCapacity=%d", c.shardCount, c.shardCapacity, int(c.shardCount)*c.shardCapacity)
	return r
}

func (c *Cache[K, V]) getShardNo[T HashableKey](key T) int {
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

func (c *cacheLRU[K, V]) moveToHead(list *listNode[K, V]) {
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

func (c *cacheLRU[K, V]) deleteNode(list *listNode[K, V]) {
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
		list.val = *new(V)
	}
}

func (c *cacheLRU[K, V]) deleteNodeByKey(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if list, ok := c.hash[key]; ok {
		c.deleteNode((list))
	}
}

func (c *cacheLRU[K, V]) get(key K) V {
	c.mu.Lock()
	defer c.mu.Unlock()
	list, ok := c.hash[key]
	if !ok {
		return *new(V)
	}

	c.moveToHead(list)

	return list.val
}

func (c *cacheLRU[K, V]) put(key K, value V) {
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

	list := &listNode[K, V]{key: key, val: value}
	c.hash[key] = list
	c.moveToHead(list)
}

func (c *Cache[K, V]) Get(key K) V {
	return c.shards[c.getShardNo(key)].get(key)
}

func (c *Cache[K, V]) Put(key K, value V) {
	c.shards[c.getShardNo(key)].put(key, value)
}

func (c *Cache[K, V]) DeleteNodeByKey(key K) {
	c.shards[c.getShardNo(key)].deleteNodeByKey(key)
}
