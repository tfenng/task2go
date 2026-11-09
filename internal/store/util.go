package store

import "github.com/google/uuid"

func NewID() string { return uuid.NewString() }

func strptr(value string) *string { return &value }

func intptr(value int) *int { return &value }
