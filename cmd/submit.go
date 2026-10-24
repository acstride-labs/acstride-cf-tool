package cmd

import "errors"

func Submit() error {
	return errors.New("Legacy submission is disabled. Use acstride-cf submit with an explicit contest, problem, language, source hash, --experimental and --confirm")
}
