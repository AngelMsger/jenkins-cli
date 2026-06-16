// Package jenkinscli is the module root. It exists only to embed packaged
// assets — the companion `jenkins` Skill — into the CLI binary, so that
// `jenkins-cli skill install` can deploy a version-matched copy regardless
// of how the binary itself was installed (npm, go install, prebuilt, source).
package jenkinscli

import "embed"

// SkillFS holds the companion Skill, rooted at "skills/jenkins".
//
//go:embed all:skills/jenkins
var SkillFS embed.FS

// SkillRoot is the path within SkillFS at which the Skill is rooted.
const SkillRoot = "skills/jenkins"
