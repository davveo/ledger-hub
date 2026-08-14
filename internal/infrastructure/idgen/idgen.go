package idgen

import (
	"strings"

	"github.com/google/uuid"
)

func New(prefix string) string {
	return prefix + strings.ReplaceAll(uuid.NewString(), "-", "")
}
