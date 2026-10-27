//go:build darwin

package desktop

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/ozine/videohelper/internal/desktop/filelock"
	"golang.org/x/sys/unix"
)

// Preserve the build's resource seal; no signing tools run on the user's machine.
//
//go:embed Info.plist
var bundleInfo []byte

//go:embed CodeResources
var bundleResources []byte

func inAppBundle(executable string) bool {
	macOS := filepath.Dir(executable)
	contents := filepath.Dir(macOS)
	return filepath.Base(macOS) == "MacOS" && filepath.Base(contents) == "Contents" && strings.HasSuffix(filepath.Dir(contents), ".app")
}

// LaunchBundle gives a single distributed executable its normal macOS app identity.
func LaunchBundle(dataDir string) (bool, error) {
	executable, err := os.Executable()
	if err != nil {
		return false, err
	}
	if mygo.IsDev() || inAppBundle(executable) {
		return false, nil
	}
	bundle, err := prepareBundle(executable, dataDir)
	if err != nil {
		return false, err
	}
	args := append([]string{"-n", "-a", bundle, "--args"}, os.Args[1:]...)
	cmd := exec.Command("/usr/bin/open", args...)
	// open(1) documents inherited environment since Tiger; no newer --env option is needed.
	cmd.Env = append(os.Environ(), "VIDEOHELPER_DATA_DIR="+dataDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		return false, fmt.Errorf("launch VideoHelper: %w: %s", err, bytes.TrimSpace(output))
	}
	return true, nil
}

func fileHash(path string) ([32]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return [32]byte{}, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return [32]byte{}, err
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum, nil
}

func prepareBundle(executable, dataDir string) (string, error) {
	launcher := filepath.Join(dataDir, "Launcher")
	if err := os.MkdirAll(launcher, 0700); err != nil {
		return "", err
	}
	lock, err := filelock.Try(filepath.Join(launcher, "update.lock"))
	if err != nil {
		return "", fmt.Errorf("prepare desktop app: %w", err)
	}
	defer lock.Close()
	bundle := filepath.Join(launcher, "VideoHelper.app")
	contents := filepath.Join(bundle, "Contents")
	sourceHash, err := fileHash(executable)
	if err != nil {
		return "", err
	}
	installedHash, hashErr := fileHash(filepath.Join(contents, "MacOS", "VideoHelper"))
	info, _ := os.ReadFile(filepath.Join(contents, "Info.plist"))
	resources, _ := os.ReadFile(filepath.Join(contents, "_CodeSignature", "CodeResources"))
	if hashErr == nil && sourceHash == installedHash && bytes.Equal(info, bundleInfo) && bytes.Equal(resources, bundleResources) {
		return bundle, nil
	}
	stage, err := os.MkdirTemp(launcher, ".install-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	stagedBundle := filepath.Join(stage, "VideoHelper.app")
	stagedContents := filepath.Join(stagedBundle, "Contents")
	for _, dir := range []string{"MacOS", "_CodeSignature"} {
		if err := os.MkdirAll(filepath.Join(stagedContents, dir), 0700); err != nil {
			return "", err
		}
	}
	if err := copyExecutable(executable, filepath.Join(stagedContents, "MacOS", "VideoHelper")); err != nil {
		return "", err
	}
	for name, data := range map[string][]byte{"Info.plist": bundleInfo, filepath.Join("_CodeSignature", "CodeResources"): bundleResources} {
		if err := os.WriteFile(filepath.Join(stagedContents, name), data, 0600); err != nil {
			return "", err
		}
	}
	if info, err := os.Lstat(bundle); errors.Is(err, os.ErrNotExist) {
		err = os.Rename(stagedBundle, bundle)
		return bundle, err
	} else if err != nil {
		return "", err
	} else if !info.IsDir() {
		return "", fmt.Errorf("desktop app path is not a directory: %s", bundle)
	}
	// Both complete directories stay intact if publication fails.
	err = unix.RenameatxNp(unix.AT_FDCWD, stagedBundle, unix.AT_FDCWD, bundle, unix.RENAME_SWAP)
	return bundle, err
}

func copyExecutable(source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("desktop executable is not a regular file")
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	return errors.Join(copyErr, out.Sync(), out.Close())
}
