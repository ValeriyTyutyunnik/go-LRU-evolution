package v2_threadsafe

import (
	"fmt"
	"log/slog"
	"sync"
)

type listNode struct {
	key  int
	val  string
	prev *listNode
	next *listNode
}

type Cache struct {
	cap  int
	mu   sync.RWMutex
	hash map[int]*listNode
	head *listNode
	tail *listNode
}

func NewCache(capacity int) *Cache {
	if capacity < 2 {
		slog.Warn("Некорректное значение capacity. инициализация кэша с capacity=2")
		capacity = 2
	}
	return &Cache{cap: capacity, hash: make(map[int]*listNode, capacity)}
}

func (c *Cache) String() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	r := fmt.Sprintf("Cache len=%d/%d", len(c.hash), c.cap)
	return r
}

func (c *Cache) moveToHead(list *listNode) {
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

func (c *Cache) deleteNode(list *listNode) {
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

func (c *Cache) DeleteNodeByKey(key int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if list, ok := c.hash[key]; ok {
		c.deleteNode((list))
	}
}

func (c *Cache) Get(key int) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	list, ok := c.hash[key]
	if !ok {
		return ""
	}

	c.moveToHead(list)

	return list.val
}

func (c *Cache) Put(key int, value string) {
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
