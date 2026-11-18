package main

import "time"

func FakeReport(id, repository, workflow, image string) Report {
	platform := PlatformTimings{Queue: 250 * time.Millisecond, Launch: 4 * time.Second, Boot: 12 * time.Second, Registration: 3 * time.Second, Ready: 8 * time.Second, JobStart: 2 * time.Second, Cleanup: 1 * time.Second}
	workload := WorkloadTimings{DependencyInstall: 18 * time.Second, Build: 24 * time.Second, Tests: 31 * time.Second, Cypress: 9 * time.Second, Docker: 14 * time.Second}
	started := time.Unix(0, 0).UTC()
	return Report{SchemaVersion: "1", ID: id, Mode: "fake", Repository: repository, Workflow: workflow, Image: image, Platform: "linux", Architecture: "x86_64", StartedAt: started, FinishedAt: started.Add(platform.Total() + workload.Total()), Duration: platform.Total() + workload.Total(), ExitCode: 0, Status: "passed", PlatformTimes: platform, WorkloadTimes: workload, Workload: defaultWorkloadEvidence("synthetic", workflow)}
}

func defaultWorkloadEvidence(provenance, workflow string) WorkloadEvidence {
	return WorkloadEvidence{ID: "leo-runners-ci", ManifestVersion: "1.0.0", ManifestDigest: "sha256:8fc0ae8b8dd1698e3f6d3566a7782062cb2f9272b97b68d9254f037a010a83ef", RepositoryCommit: "3d8acb1904854728572e7f45459255f7f2113858", ManifestWorkflow: workflow, CommandDigest: "sha256:3c964eca7067c2d505c40d3076fe152704d65c44bb7dc5d4d8b75bbbfa8b338e", Provenance: provenance, RedactionStatus: "redacted", SecretsScanned: true, RawPayloadsExcluded: true}
}
