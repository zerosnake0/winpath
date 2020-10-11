package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/mattn/go-colorable"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/sys/windows/registry"
)

const (
	userEnvPath = `Environment`
	sysEnvPath  = `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`
)

var (
	replacements = map[string]string{
		`C:\Program Files\`:       `%PF64%\`,
		`C:\Program Files (x86)\`: `%PF86%\`,
	}
)

func fillReplacements(key string) {
	v := os.Getenv(key)
	if v == "" {
		return
	}
	if !strings.HasSuffix(v, `\`) {
		v += `\`
	}
	replacements[v] = `%` + key + `%\`
}

func init() {
	fillReplacements("APPDATA")
	fillReplacements("LOCALAPPDATA")
	fillReplacements("ALLUSERSPROFILE")
}

func getEnv(name string, k registry.Key, path string) (string, error) {
	k, err := registry.OpenKey(k, path, registry.QUERY_VALUE)
	if err != nil {
		return "", err
	}
	defer k.Close()
	s, _, err := k.GetStringValue(name)
	return s, err
}

func evalEnv(name string) (string, error) {
	return os.Getenv(name), nil
	/*
		s, err := getEnv(name, registry.CURRENT_USER, userEnvPath)
		if err == nil {
			return s, nil
		}
		sysErr, ok := err.(syscall.Errno)
		if !ok || sysErr != registry.ErrNotExist {
			return "", err
		}
		return getEnv(name, registry.LOCAL_MACHINE, sysEnvPath)
	*/
}

func getPaths(k registry.Key, path string) ([]string, error) {
	s, err := getEnv("PATH", k, path)
	if err != nil {
		return nil, err
	}
	paths := strings.Split(s, ";")
	log.Debug().Msg("----- ----- ----- ----- -----")
	for _, path := range paths {
		log.Debug().Msg(path)
	}
	return paths, nil
}

func getUserPaths() ([]string, error) {
	return getPaths(registry.CURRENT_USER, userEnvPath)
}

func getSysPaths() ([]string, error) {
	return getPaths(registry.LOCAL_MACHINE, sysEnvPath)
}

func evalPath(path string) (string, error) {
	l := len(path)
	var b strings.Builder
	left := 0
	for left < l {
		c := path[left]
		if c != '%' {
			left++
			b.WriteByte(c)
			continue
		}
		right := left + 1
		for ; right < l; right++ {
			if path[right] == '%' {
				break
			}
		}
		if right == l {
			return "", fmt.Errorf("bad path %s", path)
		}
		value, err := evalEnv(path[left+1 : right])
		if err != nil {
			return "", err
		}
		subPath, err := evalPath(value)
		if err != nil {
			return "", err
		}
		b.WriteString(subPath)
		left = right + 1
	}
	return b.String(), nil
}

func processPaths(regKey registry.Key, regPath string) error {
	paths, err := getPaths(regKey, regPath)
	if err != nil {
		return err
	}
	validPaths := make([]string, 0, len(paths))
OuterLoop:
	for _, path := range paths {
		truePath, err := evalPath(path)
		if err != nil {
			return err
		}
		_, err = os.Stat(truePath)
		if err != nil {
			if os.IsNotExist(err) {
				log.Warn().Str("path", path).
					Str("true path", truePath).
					Msg("no longer exists")
				continue
			}
			log.Fatal().Err(err).Str("path", path).
				Str("true path", truePath).
				Msg("error")
		}
		for k, v := range replacements {
			if strings.HasPrefix(path, k) {
				ev := log.Info().Str("path", path).
					Str("prefix", k)
				path = v + path[len(k):]
				ev.Str("replaced", path).Msg("replaced")
				break
			}
		}
		for _, vPath := range validPaths {
			if vPath == path {
				continue OuterLoop
			}
		}
		validPaths = append(validPaths, path)
	}
	log.Info().Msg("#####")
	log.Info().Msg(strings.Join(validPaths, ";"))
	log.Info().Msg("#####")
	return nil
}

func main() {
	log.Logger = zerolog.New(zerolog.ConsoleWriter{
		Out: colorable.NewColorableStderr(),
	}).Level(zerolog.InfoLevel).With().Timestamp().Caller().Logger()

	if err := processPaths(registry.CURRENT_USER, userEnvPath); err != nil {
		log.Fatal().Err(err).Msg("unable to process user path")
	}
	if err := processPaths(registry.LOCAL_MACHINE, sysEnvPath); err != nil {
		log.Fatal().Err(err).Msg("unable to process sys path")
	}
}
