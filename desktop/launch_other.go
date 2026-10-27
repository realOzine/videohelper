//go:build !darwin

package desktop

func LaunchBundle(string) (bool, error) { return false, nil }
