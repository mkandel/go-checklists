package auth_test

import (
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/mkandel/go-checklists/internal/auth"
)

// BenchmarkHashPassword measures the cost of bcrypt.DefaultCost hashing, the
// work done once per registration/admin-create/password-reset. This bounds
// how many such requests/sec a single instance can sustain.
func BenchmarkHashPassword(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if _, err := auth.HashPassword("correct horse battery staple"); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkVerifyPassword measures the cost of the bcrypt comparison done on
// every login attempt, which the login rate limiter assumes is cheap enough
// to run per-request without becoming its own bottleneck.
func BenchmarkVerifyPassword(b *testing.B) {
	hash, err := auth.HashPassword("correct horse battery staple")
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte("correct horse battery staple")); err != nil {
			b.Fatal(err)
		}
	}
}
