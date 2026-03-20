package images

import "os"

var Registry = getRegistry()

func getRegistry() string {
	if r := os.Getenv("PACKALARES_REGISTRY"); r != "" {
		return r
	}
	return "beclab" // default for backward compatibility
}

func Ref(name, tag string) string {
	return Registry + "/" + name + ":" + tag
}
