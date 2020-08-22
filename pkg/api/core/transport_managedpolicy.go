package core

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/oslokommune/okctl/pkg/api"
)

func decodeCreateExternalSecretsPolicyRequest(_ context.Context, r *http.Request) (interface{}, error) {
	var opts api.CreateExternalSecretsPolicyOpts

	err := json.NewDecoder(r.Body).Decode(&opts)
	if err != nil {
		return nil, err
	}

	return opts, nil
}