package v6_stats

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
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
	cap   int
	mu    sync.RWMutex
	hash  map[K]*listNode[K, V]
	head  *listNode[K, V]
	tail  *listNode[K, V]
	stats cacheStats
}

type Cache[K HashableKey, V any] struct {
	shardCapacity int
	shardCount    int
	shards        []*cacheLRU[K, V]
}

func NewCache[K HashableKey, V any](shardCapacity int, shardCount int) (*Cache[K, V], error) {
	if shardCount <= 0 {
		return nil, errors.New("значение shardCount должно быть положительным числом")
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
	return fmt.Sprintf("Cache shards=%v, shardCapacity=%d, totalCapacity=%d", c.shardCount, c.shardCapacity, int(c.shardCount)*c.shardCapacity)
}

func (c *Cache[K, V]) GatherStatistic() string {
	var (
		totalStats cacheStats
		totalItems int
		sb         strings.Builder
	)
	sb.Grow(c.shardCount*120 + 200)

	for i := range c.shardCount {
		curr, total := c.shards[i].getStats()

		sb.WriteByte('[')
		sb.WriteString(strconv.Itoa(i))
		sb.WriteString("] itemsCount=")
		sb.WriteString(strconv.Itoa(total))
		sb.WriteString("; ")
		sb.WriteString(curr.String())

		totalItems += total
		totalStats.hit += curr.hit
		totalStats.miss += curr.miss
		totalStats.del += curr.del
		totalStats.upd += curr.upd
	}

	sb.WriteString("Total: itemsCount=")
	sb.WriteString(strconv.Itoa(totalItems))
	sb.WriteString("; ")
	sb.WriteString(totalStats.String())

	return sb.String()
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

func (c *cacheLRU[K, V]) getStats() (cacheStats, int) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return cacheStats{
		hit:  c.stats.hit,
		miss: c.stats.miss,
		del:  c.stats.del,
		upd:  c.stats.upd,
	}, len(c.hash)
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
		c.stats.del++
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
		c.stats.miss++
		return *new(V)
	}

	c.moveToHead(list)

	c.stats.hit++
	return list.val
}

func (c *cacheLRU[K, V]) put(key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if list, ok := c.hash[key]; ok {
		list.val = value
		c.moveToHead(list)
		c.stats.upd++
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
