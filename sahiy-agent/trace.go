package main

import (
	"fmt"
	"strings"
)

// trace — agent bitta suhbatni qanday tushunganini bosqichma-bosqich
// yozib boradi. Bir vaqtda konsolga chiqadi va dashboardda ko'rsatish
// uchun bazaga saqlanadi.
type trace struct {
	steps []string
}

// add navbatdagi qadamni qo'shadi (konsolga ham chiqaradi).
func (t *trace) add(icon, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	t.steps = append(t.steps, fmt.Sprintf("%d. %s", len(t.steps)+1, msg))
	fmt.Printf("    %s %s\n", icon, msg)
}

// String barcha qadamlarni qatorma-qator qaytaradi.
func (t *trace) String() string { return strings.Join(t.steps, "\n") }
