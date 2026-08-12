// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/hashicorp/hcl/v2"
)

func fromHCLRange(r hcl.Range) spec.SourceRange {
	return spec.SourceRange{
		Filename: r.Filename,
		Start:    fromHCLPos(r.Start),
		End:      fromHCLPos(r.End),
	}
}

func fromHCLPos(p hcl.Pos) spec.SourcePosition {
	return spec.SourcePosition{Line: p.Line, Column: p.Column, Byte: p.Byte}
}

func toHCLRange(r spec.SourceRange) hcl.Range {
	return hcl.Range{
		Filename: r.Filename,
		Start:    toHCLPos(r.Start),
		End:      toHCLPos(r.End),
	}
}

func toHCLPos(p spec.SourcePosition) hcl.Pos {
	return hcl.Pos{Line: p.Line, Column: p.Column, Byte: p.Byte}
}
