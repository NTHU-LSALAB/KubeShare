package stack

import (
	"sync"
)

type Node struct {
	value interface{}
	prev  *Node
}

type Stack struct {
	top    *Node
	length int
	lock   *sync.RWMutex
}

// create a new stack
func NewStack() *Stack {
	return &Stack{nil, 0, &sync.RWMutex{}}
}

// return the number of items in the stack
func (s *Stack) Len() int {
	return s.length
}

// view the top item on the stack
func (s *Stack) Top() interface{} {
	if s.length == 0 {
		return nil
	}

	s.lock.RLock()
	defer s.lock.RUnlock()

	return s.top.value
}

// push a value onto the top of the stack
func (s *Stack) Push(value interface{}) {
	s.lock.Lock()
	defer s.lock.Unlock()

	// curTop will be the new node's prev
	node := &Node{value, s.top}
	s.top = node
	s.length++
}

// pop the top item  of the stack and return it
func (s *Stack) Pop() interface{} {
	s.lock.Lock()
	defer s.lock.Unlock()
	if s.length == 0 {
		return nil
	}
	node := s.top
	s.top = node.prev
	s.length--
	return node.value
}
