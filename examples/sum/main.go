package main

import (
	"fmt"
	"time"
)

type User struct {
	Name string
	Age  int
}

func (u *User) Birth() int {
	return time.Now().Year() - u.Age
}

func sum(a, b int) int {
	return a + b
}

func main() {
	fmt.Println("2 + 3 =", sum(2, 3))
}

/*goit:test

import "testing"

func TestBirth(t *testing.T) {
	u := User{
		Name: "Goit",
		Age: 17,
	}

	if got, want := u.Birth(), 2009; got != want {
		t.Fatalf("birth mismatch = %d, want %d", got, want)
	}
}

func TestSum(t *testing.T) {
	if got, want := sum(2, 3), 5; got != want {
		t.Fatalf("sum(2, 3) = %d, want %d", got, want)
	}
}

func TestSumNegative(t *testing.T) {
	if got, want := sum(-2, -3), -5; got != want {
		t.Fatalf("sum(-2, -3) = %d, want %d", got, want)
	}
}

goit:end*/
