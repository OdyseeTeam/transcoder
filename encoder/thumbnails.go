package encoder

import (
	"fmt"
	"os/exec"
)

const interval = "2" // seconds
const width = "300"
const height = "200"
const columns = "10"

type ThumbnailGenerator struct {
	cmdPath string
	args    []string
}

type ThumbnailGeneratorProgress struct {
}

func NewThumbnailGenerator(cmdPath string) (*ThumbnailGenerator, error) {
	cmd := exec.Command(cmdPath, "-h")
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("unable to execute generator: %w", err)
	}
	return &ThumbnailGenerator{cmdPath, []string{interval, width, height, columns}}, nil
}

func (g ThumbnailGenerator) Generate(input, output string) error {
	args := append([]string{input}, g.args...)
	args = append(args, output)
	_, err := exec.Command(g.cmdPath, args...).Output()
	if err != nil {
		return err
	}
	return nil
}
