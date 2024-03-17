package nodejs

import (
	_ "embed"

	"github.com/KarpelesLab/pjson"
)

//go:embed bootstrap.js
var bootstrapJs string

var bootstrapEnc, _ = pjson.Marshal(bootstrapJs)
