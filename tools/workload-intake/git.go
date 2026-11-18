package main

import (
	"fmt"
	"os/exec"
	"strings"
)

func gitOutput(repo string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(output)), nil
}

func verifyCheckout(repo, commit string) error {
	head, err := gitOutput(repo, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return err
	}
	if head != commit {
		return fmt.Errorf("checkout HEAD %s does not match pinned commit", head)
	}
	status, err := gitOutput(repo, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return err
	}
	if status != "" {
		return fmt.Errorf("checkout is dirty (%d status bytes)", len(status))
	}
	return nil
}
