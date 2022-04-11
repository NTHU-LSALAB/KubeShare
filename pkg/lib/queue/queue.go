package queue

import (
	"sync"
)

type Node struct {
	value interface{}
	next  *Node
}

type Queue struct {
	front  *Node
	back   *Node
	length int
	lock   *sync.RWMutex
}

func NewQueue() *Queue {
	return &Queue{nil, nil, 0, &sync.RWMutex{}}
}

func (q *Queue) Front() interface{} {
	if q.length == 0 {
		return nil
	}

	q.lock.RLock()
	defer q.lock.RUnlock()

	return q.front.value
}

func (q *Queue) Enqueue(value interface{}) {
	q.lock.Lock()
	defer q.lock.Unlock()
	node := &Node{value, nil}

	if q.length == 0 {
		q.front = node
		q.back = node
		q.length++
		return
	}

	q.back.next = node
	q.back = node
	q.length++
}

func (q *Queue) Dequeue() interface{} {
	q.lock.Lock()
	defer q.lock.Unlock()

	if q.length == 0 {
		return nil
	}
	node := q.front
	q.front = q.front.next
	q.length--
	return node.value
}

func (q *Queue) Len() int {
	return q.length
}
