package promptset

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
)

const maxPromptBytes = 1 << 20 // 1 MiB

type Options struct {
	Mutate      bool
	MaxVariants int // max variants per seed (including the original); <= 0 means "no limit"
}

func Stream(ctx context.Context, path string, out chan<- string, opt Options) error {
	r, closeFn, err := openPath(path)
	if err != nil {
		return err
	}
	if closeFn != nil {
		defer closeFn()
	}

	sc := bufio.NewScanner(r)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, maxPromptBytes)

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !opt.Mutate {
			if err := send(ctx, out, line); err != nil {
				return err
			}
			continue
		}
		variants := Mutate(line, opt.MaxVariants)
		for _, v := range variants {
			if err := send(ctx, out, v); err != nil {
				return err
			}
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("read prompts: %w", err)
	}
	return nil
}

func send(ctx context.Context, out chan<- string, prompt string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case out <- prompt:
		return nil
	}
}

func openPath(path string) (r io.Reader, closeFn func() error, err error) {
	if path == "-" {
		return os.Stdin, nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open prompts file: %w", err)
	}
	return f, f.Close, nil
}
