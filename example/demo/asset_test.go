package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The animation in the README is generated, and a generated file that nobody
// checks goes stale quietly: the one this replaced was a screen recording of
// somebody's desktop, three months and forty fixes out of date, with no way to
// tell from the repository that it no longer showed what the library does.
//
// What is checked here is that the committed animation is the one the committed
// inputs produce. The animation itself cannot be regenerated during a test --
// VHS drives a real terminal through ttyd and ffmpeg, neither of which is a
// test dependency, and a GIF's bytes depend on the machine that encoded it --
// so the check is a lock file written at the moment the animation was made,
// holding the digest of the tape, of the program the tape drives, and of the
// result. Editing any of the three without running `make demo` fails here.
//
// The tape's own dimensions are compared with the GIF's header as well, which
// ties the artifact to its source without going through the lock.

const (
	tapeFile = "demo.tape"
	progFile = "main.go"
	gifFile  = "../../doc/img/demo.gif"
	lockFile = "demo.lock"
	readme   = "../../README.md"
)

// lockedFiles is what the lock covers, in the order it lists them.
//
// The two text files are hashed with their line endings normalized, because a
// checkout is allowed to rewrite them: a Windows runner with git's autocrlf
// turned on hands the test CRLF for files that were committed with LF, and the
// digest of a file that nobody edited would not match. The GIF is hashed as it
// is -- git leaves binary files alone, and normalizing one would corrupt it.
var lockedFiles = []struct {
	name string
	text bool
}{
	{tapeFile, true},
	{progFile, true},
	{gifFile, false},
}

var update = flag.Bool("update", false, "rewrite demo.lock from the files on disk, after regenerating the animation")

func TestDemoGIFIsCurrent(t *testing.T) {
	if *update {
		if err := writeLock(); err != nil {
			t.Fatalf("rewriting %s: %v", lockFile, err)
		}
		t.Logf("%s rewritten", lockFile)
		return
	}

	locked, err := readLock()
	if err != nil {
		t.Fatalf("reading %s: %v", lockFile, err)
	}

	for _, file := range lockedFiles {
		want, ok := locked[file.name]
		if !ok {
			t.Errorf("%s does not cover %s; run `make demo`", lockFile, file.name)
			continue
		}
		got, err := digest(file.name, file.text)
		if err != nil {
			t.Errorf("hashing %s: %v", file.name, err)
			continue
		}
		if got != want {
			t.Errorf("%s has changed since the animation was recorded (%s is %s, %s says %s).\n"+
				"The animation in the README no longer shows what this program does. Regenerate it with `make demo`.",
				file.name, file.name, got[:12], lockFile, want[:12])
		}
	}
}

// TestDemoGIFMatchesTheTape compares the recorded animation with the size the
// tape asks for, which is a check on the artifact rather than on the
// bookkeeping: a GIF copied in from somewhere else fails it even if the lock
// was rewritten.
func TestDemoGIFMatchesTheTape(t *testing.T) {
	t.Parallel()

	header := make([]byte, 10)
	file, err := os.Open(gifFile)
	if err != nil {
		t.Fatalf("opening the animation: %v", err)
	}
	defer file.Close()
	if _, err := file.Read(header); err != nil {
		t.Fatalf("reading the animation's header: %v", err)
	}

	// A GIF starts with "GIF87a" or "GIF89a", then the logical screen width and
	// height as little-endian 16-bit numbers.
	if magic := string(header[:6]); magic != "GIF89a" && magic != "GIF87a" {
		t.Fatalf("%s does not start like a GIF: %q", gifFile, magic)
	}
	gotWidth := int(binary.LittleEndian.Uint16(header[6:8]))
	gotHeight := int(binary.LittleEndian.Uint16(header[8:10]))

	wantWidth, wantHeight, err := tapeSize()
	if err != nil {
		t.Fatalf("reading the tape's size: %v", err)
	}
	if gotWidth != wantWidth || gotHeight != wantHeight {
		t.Errorf("the animation is %dx%d and %s asks for %dx%d; regenerate it with `make demo`",
			gotWidth, gotHeight, tapeFile, wantWidth, wantHeight)
	}
}

// TestReadmeShowsTheAnimation keeps the asset and the page that uses it
// together: an animation nothing references is one nobody will notice going
// stale.
func TestReadmeShowsTheAnimation(t *testing.T) {
	t.Parallel()

	content, err := os.ReadFile(readme)
	if err != nil {
		t.Fatalf("reading the README: %v", err)
	}
	if want := "doc/img/demo.gif"; !strings.Contains(string(content), want) {
		t.Errorf("the README does not reference %s", want)
	}
}

// TestDigestIgnoresTheLineEndingsACheckoutChose covers the reason the text
// files are normalized before they are hashed: a Windows checkout with git's
// autocrlf turned on rewrites them, and a digest taken over the bytes on disk
// would report a file nobody edited as changed. A binary file is hashed as it
// is, because git does not rewrite one and normalizing it would corrupt it.
func TestDigestIgnoresTheLineEndingsACheckoutChose(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	unix := filepath.Join(dir, "unix")
	windows := filepath.Join(dir, "windows")
	if err := os.WriteFile(unix, []byte("Set Width 1100\nSet Height 720\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(windows, []byte("Set Width 1100\r\nSet Height 720\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	asText := func(name string) string {
		t.Helper()
		sum, err := digest(name, true)
		if err != nil {
			t.Fatal(err)
		}
		return sum
	}
	if asText(unix) != asText(windows) {
		t.Error("the same text hashes differently depending on the line endings the checkout chose")
	}

	asBytes := func(name string) string {
		t.Helper()
		sum, err := digest(name, false)
		if err != nil {
			t.Fatal(err)
		}
		return sum
	}
	if asBytes(unix) == asBytes(windows) {
		t.Error("a file hashed as binary is being normalized")
	}
}

// tapeSize returns the width and height the tape sets.
func tapeSize() (width, height int, err error) {
	content, err := os.ReadFile(tapeFile)
	if err != nil {
		return 0, 0, err
	}
	for _, field := range []struct {
		name string
		into *int
	}{{"Width", &width}, {"Height", &height}} {
		pattern := regexp.MustCompile(`(?m)^Set\s+` + field.name + `\s+(\d+)\s*$`)
		match := pattern.FindSubmatch(content)
		if match == nil {
			return 0, 0, fmt.Errorf("%s sets no %s", tapeFile, field.name)
		}
		value, convErr := strconv.Atoi(string(match[1]))
		if convErr != nil {
			return 0, 0, convErr
		}
		*field.into = value
	}
	return width, height, nil
}

func digest(name string, text bool) (string, error) {
	content, err := os.ReadFile(filepath.Clean(name))
	if err != nil {
		return "", err
	}
	if text {
		content = bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
	}
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:]), nil
}

func readLock() (map[string]string, error) {
	content, err := os.ReadFile(lockFile)
	if err != nil {
		return nil, err
	}
	locked := make(map[string]string)
	for line := range strings.SplitSeq(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, sum, ok := strings.Cut(line, " ")
		if !ok {
			return nil, fmt.Errorf("%s: cannot read the line %q", lockFile, line)
		}
		locked[name] = strings.TrimSpace(sum)
	}
	return locked, nil
}

func writeLock() error {
	var b strings.Builder
	b.WriteString("# Written by `make demo`. Do not edit by hand.\n")
	b.WriteString("# The digests of the animation in the README and of the tape and program it\n")
	b.WriteString("# was recorded from. TestDemoGIFIsCurrent fails when they no longer agree.\n")
	for _, file := range lockedFiles {
		sum, err := digest(file.name, file.text)
		if err != nil {
			return err
		}
		fmt.Fprintf(&b, "%s %s\n", file.name, sum)
	}
	return os.WriteFile(lockFile, []byte(b.String()), 0o600)
}
