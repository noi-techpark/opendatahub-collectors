// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: CC0-1.0

package ssim_test

import (
	ssim "com/opendatahub/ssimparser"
	"os"
	"testing"

	"gotest.tools/v3/assert"
)

func TestParser_Parse(t *testing.T) {
	f, err := os.Open("../testdata/sample.ssim")
	assert.NilError(t, err)
	defer f.Close()

	p := ssim.NewParser()
	s, err := p.Parse(f)
	assert.NilError(t, err)
	assert.Assert(t, s != nil)
}
