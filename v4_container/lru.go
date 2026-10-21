package v4_container

import (
	"container/list"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/ValeriyTyutyunnik/go-lru-evolution/buckets"
)

type kv struct {
	key int
	val string
}

type cacheLRU struct {
	cap  int
	mu   sync.RWMutex
	hash map[int]*list.Element
	list *list.List
}

type Cache struct {
	shardCapacity int
	shardCount    int
	shards        []*cacheLRU
}

func NewCache(shardCapacity int, shardCount int) (*Cache, error) {
	if shardCount <= 0 {
		return nil, errors.New("значение shardCount должно быть положительным числом")
	}
	if shardCapacity < 2 {
		slog.Warn("Некорректное значение capacity. инициализация кэша с shardCapacity=2")
		shardCapacity = 2
	}
	res := &Cache{shardCapacity: shardCapacity, shardCount: shardCount}
	res.shards = make([]*cacheLRU, shardCount)
	for i := range shardCount {
		res.shards[i] = &cacheLRU{cap: shardCapacity, hash: make(map[int]*list.Element, shardCapacity), list: list.New()}
	}

	return res, nil
}

func (c *Cache) String() string {
	r := fmt.Sprintf("Cache shards=%v, shardCapacity=%d, totalCapacity=%d", c.shardCount, c.shardCapacity, int(c.shardCount)*c.shardCapacity)
	return r
}

func (c *Cache) getShardNo(key int) int {
	return int(buckets.For64(uint64(key), uint64(c.shardCount)))
}

func (c *cacheLRU) deleteNodeByKey(key int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.hash[key]; ok {
		c.list.Remove(elem)
		delete(c.hash, key)
	}
}

func (c *cacheLRU) get(key int) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	elem, ok := c.hash[key]
	if !ok {
		return ""
	}

	c.list.MoveToFront(elem)
	if item, ok := elem.Value.(*kv); ok {
		return item.val
	}
	return ""
}

func (c *cacheLRU) put(key int, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.hash[key]; ok {
		elem.Value = &kv{key: key, val: value}
		c.list.MoveToFront(elem)
		return
	}

	if len(c.hash) >= c.cap {
		elem := c.list.Back()
		if elem != nil {
			if item, ok := elem.Value.(*kv); ok {
				delete(c.hash, item.key)
			}
			c.list.Remove(elem)
		}
	}

	elem := c.list.PushFront(&kv{key: key, val: value})
	c.hash[key] = elem
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
