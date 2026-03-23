package deploy

import "embed"

//go:embed all:crds all:platform all:framework all:proxy all:apps all:infrastructure all:envoy
var Manifests embed.FS
