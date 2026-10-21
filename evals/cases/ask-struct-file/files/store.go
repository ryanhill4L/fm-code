package main

type Store struct{ users []User }

func NewStore() *Store { return &Store{} }

func (s *Store) Add(u User) { s.users = append(s.users, u) }

func (s *Store) Count() int { return len(s.users) }
