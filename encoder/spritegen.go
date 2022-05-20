package encoder

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/lbryio/transcoder/pkg/logging"
)

type SpriteGenerator struct {
	cmdPath string
	args    []string
	log     logging.KVLogger
}

// defaultArgs contains arguments to nodejs plus script args.
var defaultArgs = []string{
	"/usr/src/spritegen/cli.js",
	"--interval", "2",
	"--filename", "stream",
}

func NewSpriteGenerator(cmdPath string, log logging.KVLogger) (*SpriteGenerator, error) {
	cmd := exec.Command(cmdPath, "-h")
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("unable to execute generator: %w", err)
	}
	return &SpriteGenerator{cmdPath, defaultArgs, log}, nil
}

func (g SpriteGenerator) Generate(input, output string) error {
	args := append(g.args, "--input", input)
	args = append(args, "--outputFolder", output)
	g.log.Info("starting spritegen",
		"cmd", g.cmdPath,
		"args", strings.Join(args, " "),
	)
	_, err := exec.Command(g.cmdPath, args...).Output()
	if err != nil {
		return err
	}
	return nil
}
