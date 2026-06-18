package auth

import "golang.org/x/crypto/bcrypt"

// HashPassword returns a bcrypt hash suitable for storage in users.password_hash.
func HashPassword(plaintext string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// CheckPassword reports whether plaintext matches the stored bcrypt hash.
// An empty stored hash (user never set a password) always fails.
func CheckPassword(hash, plaintext string) bool {
	if hash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plaintext)) == nil
}
