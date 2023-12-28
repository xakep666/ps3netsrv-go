package kongini

import (
	"fmt"
	"io"
	"strings"

	"github.com/alecthomas/kong"
	"gopkg.in/ini.v1"
)

func Loader(r io.Reader) (kong.Resolver, error) {
	iniFile, err := ini.Load(r)
	if err != nil {
		return nil, fmt.Errorf("error loading ini file: %w", err)
	}

	return kong.ResolverFunc(func(kctx *kong.Context, parent *kong.Path, flag *kong.Flag) (any, error) {
		var path []string
		for n := parent.Node(); n != nil && n.Type != kong.ApplicationNode; n = n.Parent {
			path = append([]string{n.Name}, path...)
		}
		path = append(path, flag.Name)

		sectionName := ini.DefaultSection
		keyName := strings.Join(path, ".")
		if i := strings.LastIndexByte(keyName, '.'); i != -1 {
			sectionName = keyName[:i]
			keyName = keyName[i+1:]
		}

		section, err := iniFile.GetSection(sectionName)
		if err != nil {
			section, err = iniFile.GetSection(ini.TitleUnderscore(sectionName))
		}
		if err != nil {
			return nil, nil // not found
		}

		key, err := section.GetKey(keyName)
		if err != nil {
			key, err = section.GetKey(ini.TitleUnderscore(keyName))
		}
		if err != nil {
			return nil, nil // not found
		}

		return key.Value(), nil
	}), nil
}

var _ kong.ConfigurationLoader = Loader
