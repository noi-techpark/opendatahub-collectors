// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: CC0-1.0

package ssim2gtfs_test

import (
	"os"
	"testing"

	"gotest.tools/v3/assert"
	"opendatahub.com/ssim2gtfs"
	ssim "opendatahub.com/ssimparser"
)

func TestSSIMToGTFSConverter_Convert(t *testing.T) {
	f, err := os.Open("../testdata/sample.ssim")
	assert.NilError(t, err)
	defer f.Close()

	p := ssim.NewParser()
	s, err := p.Parse(f)
	assert.NilError(t, err)
	assert.Assert(t, s != nil)

	c := ssim2gtfs.NewSSIMToGTFSConverter("Skyalps", "https://skyalps.com", "UTC")

	gotErr := c.Convert(s, "gtfs.zip")
	assert.NilError(t, gotErr)
}
