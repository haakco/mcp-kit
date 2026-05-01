package cliauth

import "github.com/haakco/mcp-kit/oauth"

type pkcePair struct {
	Verifier  string
	Challenge string
}

func newPKCEPair() (pkcePair, error) {
	pair, err := oauth.NewPKCEPair()
	if err != nil {
		return pkcePair{}, err
	}
	return pkcePair{Verifier: pair.Verifier, Challenge: pair.Challenge}, nil
}

func randomState() (string, error) {
	return oauth.RandomState()
}
