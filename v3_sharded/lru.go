package v3_sharded

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/ValeriyTyutyunnik/go-lru-evolution/buckets"
)

type listNode struct {
	key  int
	val  string
	prev *listNode
	next *listNode
}

type cacheLRU struct {
	cap  int
	mu   sync.RWMutex
	hash map[int]*listNode
	head *listNode
	tail *listNode
}

type Cache struct {
	shardCapacity int
	shardCount    uint32
	shards        []*cacheLRU
}

func NewCache(shardCapacity int, shardCount uint32) (*Cache, error) {
	if shardCount == 0 || (shardCount&(shardCount-1)) != 0 {
		return nil, errors.New("значение shardCount должно быть степенью двойки")
	}
	if shardCapacity < 2 {
		slog.Warn("Некорректное значение capacity. инициализация кэша с shardCapacity=2")
		shardCapacity = 2
	}
	res := &Cache{shardCapacity: shardCapacity, shardCount: shardCount}
	res.shards = make([]*cacheLRU, shardCount)
	for i := range shardCount {
		res.shards[i] = &cacheLRU{cap: shardCapacity, hash: make(map[int]*listNode, shardCapacity)}
	}

	return res, nil
}

func (c *Cache) String() string {
	r := fmt.Sprintf("Cache shards=%v, shardCapacity=%d, totalCapacity=%d", c.shardCount, c.shardCapacity, int(c.shardCount)*c.shardCapacity)
	return r
}

func (c *Cache) getShardNo(key int) int {
	return int(buckets.For32(uint32(key), c.shardCount))
}

func (c *cacheLRU) moveToHead(list *listNode) {
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

func (c *cacheLRU) deleteNode(list *listNode) {
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

func (c *cacheLRU) deleteNodeByKey(key int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if list, ok := c.hash[key]; ok {
		c.deleteNode((list))
	}
}

func (c *cacheLRU) get(key int) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	list, ok := c.hash[key]
	if !ok {
		return ""
	}

	c.moveToHead(list)

	return list.val
}

func (c *cacheLRU) put(key int, value string) {
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

	list := &listNode{key: key, val: value}
	c.hash[key] = list
	c.moveToHead(list)
}

func (c *Cache) Get(key int) string {
	return c.shards[c.getShardNo(key)].get(key)
}

func (c *Cache) Put(key int, value string) {
	c.shards[c.getShardNo(key)].put(key, value)
}

func (c *Cache) DeleteNodeByKey(key int) {
	c.shards[c.getShardNo(key)].deleteNodeByKey(key)
}
