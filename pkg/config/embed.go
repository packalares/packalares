package config

import _ "embed"

//go:embed config.yaml.template
var ConfigTemplate string
