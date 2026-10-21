package main

type Stack struct{ items []int }

func (s *Stack) Push(v int) { s.items = append(s.items, v) }

func (s *Stack) Pop() int {
	v := s.items[0]
	s.items = s.items[1:]
	return v
}

func (s *Stack) Len() int { return len(s.items) }
