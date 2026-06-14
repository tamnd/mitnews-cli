package cli

import (
	"errors"

	"github.com/tamnd/mitnews-cli/mitnews"
)

func isNotFound(err error) bool {
	return errors.Is(err, mitnews.ErrUnknownTopic)
}

func mapFetchErr(err error) error {
	if err == nil {
		return nil
	}
	if isNotFound(err) {
		return codeError(exitUsage, err)
	}
	return codeError(exitError, err)
}
