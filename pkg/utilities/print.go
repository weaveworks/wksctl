package utilities

import (
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

func PrintErrors(errors field.ErrorList) {
	for _, e := range errors {
		log.Errorf("%v\n", e)
	}
}
