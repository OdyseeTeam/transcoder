package encoder

import (
	"fmt"
	"os/exec"
)

type SpriteGenerator struct {
	cmdPath string
	args    []string
}

// defaultArgs contains arguments to nodejs plus script args.
var defaultArgs = []string{
	"/usr/src/spritegen/cli.js",
	"--interval", "2",
	"--filename", "spr",
}

func NewSpriteGenerator(cmdPath string) (*SpriteGenerator, error) {
	cmd := exec.Command(cmdPath, "-h")
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("unable to execute generator: %w", err)
	}
	return &SpriteGenerator{cmdPath, defaultArgs}, nil
}

func (g SpriteGenerator) Generate(input, output string) error {
	args := append(g.args, "--input", input)
	args = append(args, "--outputFolder", output)
	_, err := exec.Command(g.cmdPath, args...).Output()
	if err != nil {
		return err
	}
	return nil
}
