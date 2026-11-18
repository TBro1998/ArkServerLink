//go:build !windows

package auth

func protect(plain []byte) ([]byte, error) { return plain, nil }

func unprotect(data []byte) ([]byte, error) { return data, nil }
