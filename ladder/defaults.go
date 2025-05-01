package ladder

import _ "embed"

//go:embed defaults.yml
var defaultLadderYaml []byte

const DefaultCRF = 24

var Default, _ = Load(defaultLadderYaml)
