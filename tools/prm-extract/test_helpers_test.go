package main

import "os"

func writeFile(path, body string) error {
	return os.WriteFile(path, []byte(body), 0o644)
}

func makeExecutable(path string) error {
	return os.Chmod(path, 0o755)
}
