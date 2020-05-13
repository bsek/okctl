package digest_test

import (
	"io"
	"strings"
	"testing"

	"github.com/oslokommune/okctl/pkg/digest"
	"github.com/stretchr/testify/assert"
)

func TestDigests(t *testing.T) {
	testCases := []struct {
		name      string
		digester  digest.Digester
		content   io.Reader
		expect    interface{}
		expectErr bool
	}{
		{
			name:     "No digests",
			digester: digest.NewDigester(),
			expect:   map[digest.DigestType]string{},
			content:  strings.NewReader("some content"),
		},
		{
			name:     "Verify multiple digests",
			digester: digest.NewDigester(digest.DigestTypeMD5, digest.DigestTypeSHA1, digest.DigestTypeSHA256, digest.DigestTypeSHA512),
			expect: map[digest.DigestType]string{
				digest.DigestTypeMD5:    "736db904ad222bf88ee6b8d103fceb8e",
				digest.DigestTypeSHA1:   "5ec1a3cb71c75c52cf23934b137985bd2499bd85",
				digest.DigestTypeSHA256: "373993310775a34f5ad48aae265dac65c7abf420dfbaef62819e2cf5aafc64ca",
				digest.DigestTypeSHA512: "47bb28d146567b3be18d06d8468aaa8222183fe6b2a942b17b6a48bbc32bda7213f7dc1acf36677f7710cffa7add3f3656597630bf0d591f34145015f59724e1",
			},
			content: strings.NewReader("this is some content"),
		},
		{
			name:      "Unknown digester",
			digester:  digest.NewDigester("fake"),
			expect:    "unsupported digester: fake",
			expectErr: true,
			content:   strings.NewReader("hi"),
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.digester.Digest(tc.content)

			if tc.expectErr {
				assert.Equal(t, tc.expect, err.Error())
			} else {
				assert.Nil(t, err)
				assert.Equal(t, tc.expect, got)
			}
		})
	}
}
