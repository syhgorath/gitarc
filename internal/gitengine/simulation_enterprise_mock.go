package gitengine

import (
	"fmt"

	"gitarc/internal/model"
)

// buildEnterpriseSimulation creates a large-scale repository simulation representing
// an enterprise engineering organization with 4 primary tracks and 50 active developers.
func buildEnterpriseSimulation() simWorkspace {
	// 50 Developers Roster:
	// Alice Chen, Mehmet Yilmaz, David Kim, Elena Rostova, Carlos Silva, Sarah Jenkins,
	// Kenji Sato, Fatima Zahra, Lucas Muller, Ayse Kaya, Priya Patel, Marcus Vance,
	// Emily Blunt, Tarik Ozkan, Olga Ivanova, Chen Wei, Diego Torres, Anna Kowalski,
	// Hassan Ali, Kevin Duque, Rachel Green, Zeynep Demir, Brian Cox, Sophie Martin,
	// Alex Turner, Cem Yilmaz, Victoria Beck, Leo Messi, Elizabeth Swan, Mustafa Ak,
	// John Doe, Jane Smith, Can Erik, Gita Rao, Tom Hanks, Deniz Oz,
	// Charlie Brown, Selin Erdogan, Robert De, Alperen Sen, Clara Oswald, Baris Manco,
	// Emma Watson, Serdar Oz, Natalie Portman, Onur Bay, Yuki Tsunoda, Burak Oz,
	// Mia Wallace, Hakan Sukru.

	nodes := []model.CommitNode{
		{SHA: "a101f01", ShortSHA: "a101f01", Subject: "chore(release): cut v3.0.0 enterprise lts baseline", Author: "alice.chen", RelativeTime: "14 gün önce", SpineIndex: 0, Tag: "v3.0.0"},
		{SHA: "b202e12", ShortSHA: "b202e12", Subject: "feat(gateway): envoy proxy mesh ingress integration", Author: "david.kim", RelativeTime: "12 gün önce", SpineIndex: 1},
		{SHA: "c303d23", ShortSHA: "c303d23", Subject: "Merge PR #790 feat/distributed-cache-redis7", Author: "mehmet.yilmaz", RelativeTime: "11 gün önce", SpineIndex: 2, IsMerge: true},
		{SHA: "d404c34", ShortSHA: "d404c34", Subject: "chore(security): apply cve-2026-4190 kernel patch", Author: "elena.rostova", RelativeTime: "10 gün önce", SpineIndex: 3, Tag: "v3.1.0"},
		{SHA: "e505b45", ShortSHA: "e505b45", Subject: "Merge PR #794 fix/jwt-rsa-key-rotation", Author: "carlos.silva", RelativeTime: "9 gün önce", SpineIndex: 4, IsMerge: true},
		{SHA: "f606a56", ShortSHA: "f606a56", Subject: "feat(observability): opentelemetry trace collector", Author: "sarah.jenkins", RelativeTime: "8 gün önce", SpineIndex: 5},
		{SHA: "0707967", ShortSHA: "0707967", Subject: "Merge PR #798 chore/upgrade-go123", Author: "kenji.sato", RelativeTime: "7 gün önce", SpineIndex: 6, IsMerge: true},
		{SHA: "1808878", ShortSHA: "1808878", Subject: "chore(release): v3.2.0 platform core rollout", Author: "fatima.zahra", RelativeTime: "6 gün önce", SpineIndex: 7, Tag: "v3.2.0"},
		{SHA: "2909789", ShortSHA: "2909789", Subject: "feat(billing): multi-currency stripe webhooks handler", Author: "priya.patel", RelativeTime: "5 gün önce", SpineIndex: 8},
		{SHA: "3a0a69a", ShortSHA: "3a0a69a", Subject: "Merge PR #802 feat/kafka-event-streaming", Author: "lucas.muller", RelativeTime: "4 gün önce", SpineIndex: 9, IsMerge: true},
		{SHA: "4b0b5ab", ShortSHA: "4b0b5ab", Subject: "fix(db): postgres deadlocks on concurrent audit writes", Author: "ayse.kaya", RelativeTime: "3 gün önce", SpineIndex: 10},
		{SHA: "5c0c4bc", ShortSHA: "5c0c4bc", Subject: "Merge PR #807 perf/graphql-query-batching", Author: "marcus.vance", RelativeTime: "2 gün önce", SpineIndex: 11, IsMerge: true},
		{SHA: "6d0d3cd", ShortSHA: "6d0d3cd", Subject: "feat(auth): webauthn passkey biometric authentication", Author: "emily.blunt", RelativeTime: "1 gün önce", SpineIndex: 12},
		{SHA: "7e0e2de", ShortSHA: "7e0e2de", Subject: "Merge PR #810 fix/k8s-pod-disruption-budget", Author: "tarik.ozkan", RelativeTime: "18 saat önce", SpineIndex: 13, IsMerge: true, Tag: "v3.3.0-rc2"},
		{SHA: "8f0f1ef", ShortSHA: "8f0f1ef", Subject: "chore(config): bump production cluster replicas to 64", Author: "olga.ivanova", RelativeTime: "8 saat önce", SpineIndex: 14},
		{SHA: "90100f0", ShortSHA: "90100f0", Subject: "ci(security): enforce codeql & gitleaks scanner", Author: "chen.wei", RelativeTime: "2 saat önce", SpineIndex: 15, IsHEAD: true},
	}

	entCiFailLog := `2026-11-17T06:14:02.100Z ##[group]Testing service mesh tracing spans
2026-11-17T06:14:04.300Z === RUN   TestDistributedTracePropagation
2026-11-17T06:14:05.120Z --- PASS: TestDistributedTracePropagation (0.82s)
2026-11-17T06:14:05.121Z === RUN   TestOpenTelemetrySpanExporter
2026-11-17T06:14:07.450Z     exporter_test.go:88: context deadline exceeded after 2000ms waiting for OTLP gRPC endpoint: 10.240.12.9:4317
2026-11-17T06:14:07.452Z --- FAIL: TestOpenTelemetrySpanExporter (2.33s)
2026-11-17T06:14:07.455Z === RUN   TestSpanPanicRecovery
2026-11-17T06:14:07.460Z panic: nil pointer dereference evaluating spanContext.TraceFlags()
2026-11-17T06:14:07.461Z goroutine 148 [running]:
2026-11-17T06:14:07.462Z platform/internal/telemetry.ExportSpan(...)
2026-11-17T06:14:07.463Z 	services/telemetry/exporter.go:142 +0x2a8
2026-11-17T06:14:07.470Z FAIL	platform/services/telemetry	3.150s
2026-11-17T06:14:07.480Z ##[error]Process completed with exit code 1.`

	entConflictPreview := `<<<<<< HEAD (main @ 8f0f1ef)
replicas: 64
minReplicas: 16
autoscaling:
  targetCPUUtilizationPercentage: 70
  targetMemoryUtilizationPercentage: 75
======
replicas: 32
minReplicas: 8
autoscaling:
  targetCPUUtilizationPercentage: 80
  targetMemoryUtilizationPercentage: 85
>>>>>> origin/feat/auth-v2-jwt-rotation

[git merge-tree --write-tree] Otomatik Çözüm Önerisi:
⚠ Çakışan dosya: deploy/helm/values.yaml (Üretim küme autoscaling eşikleri)`

	branches := []model.BranchArc{
		// 1. DÖRT ANA DAL (Primary Long-Running Tracks)
		// Track 1: develop (Ana geliştirme entegrasyon dalı)
		{
			ID:              "br-track-develop",
			Name:            "develop",
			Hemisphere:      "upper",
			State:           "open",
			StartSpineIndex: 10,
			EndSpineIndex:   15,
			BaseSHA:         "4b0b5ab",
			HasRemote:       true,
			RemoteRef:       "origin/develop",
			PR: &model.PRStatus{
				Number:      770,
				Title:       "track(develop): active platform integration line",
				Author:      "diego.torres",
				ReviewState: "APPROVED",
				CI:          &model.CIStatus{State: "SUCCESS", Workflow: "Nightly Integration Test", Duration: "4m 12s"},
			},
			HasLocal:         true,
			LocalSHA:         "fa0192b",
			Author:           "diego.torres",
			AheadMain:        5,
			BehindMain:       1,
			UncommittedCount: 0,
			LastCommitMsg:    "develop: sync nightly service builds across all clusters",
			ChangedFiles:     []string{"services/develop-manifest.yaml"},
		},
		// Track 2: staging (Ön-canlı doğrulama dalı)
		{
			ID:              "br-track-staging",
			Name:            "staging",
			Hemisphere:      "upper",
			State:           "open",
			StartSpineIndex: 12,
			EndSpineIndex:   15,
			BaseSHA:         "6d0d3cd",
			HasRemote:       true,
			RemoteRef:       "origin/staging",
			PR: &model.PRStatus{
				Number:      775,
				Title:       "track(staging): pre-production validation track",
				Author:      "anna.kowalski",
				ReviewState: "APPROVED",
				CI:          &model.CIStatus{State: "SUCCESS", Workflow: "E2E Smoke Tests", Duration: "3m 45s"},
			},
			HasLocal:         true,
			LocalSHA:         "fb1283c",
			Author:           "anna.kowalski",
			AheadMain:        3,
			BehindMain:       0,
			UncommittedCount: 0,
			LastCommitMsg:    "staging: promote RC candidates to stg-us-east-2 cluster",
			ChangedFiles:     []string{"deploy/staging/kustomization.yaml"},
		},
		// Track 3: release/v3.3 (Sürüm hazırlık dalı)
		{
			ID:              "br-track-release-33",
			Name:            "release/v3.3",
			Hemisphere:      "upper",
			State:           "open",
			StartSpineIndex: 13,
			EndSpineIndex:   15,
			BaseSHA:         "7e0e2de",
			HasRemote:       true,
			RemoteRef:       "origin/release/v3.3",
			PR: &model.PRStatus{
				Number:      780,
				Title:       "track(release): v3.3.0 stabilization and release gate",
				Author:      "hassan.ali",
				ReviewState: "REVIEW_REQUIRED",
				CI:          &model.CIStatus{State: "RUNNING", Workflow: "Release Candidate Sanity", Duration: "1m 15s"},
			},
			HasLocal:         true,
			LocalSHA:         "fc2374d",
			Author:           "hassan.ali",
			AheadMain:        2,
			BehindMain:       0,
			UncommittedCount: 0,
			LastCommitMsg:    "release: lock dependency freeze for v3.3.0 final tag",
			ChangedFiles:     []string{"VERSION", "CHANGELOG.md"},
		},
		// Track 4: support/lts-2.x (Uzun vadeli destek dalı)
		{
			ID:              "br-track-lts-2x",
			Name:            "support/lts-2.x",
			Hemisphere:      "upper",
			State:           "open",
			StartSpineIndex: 0,
			EndSpineIndex:   7,
			BaseSHA:         "a101f01",
			HasRemote:       true,
			RemoteRef:       "origin/support/lts-2.x",
			PR: &model.PRStatus{
				Number:      750,
				Title:       "track(lts): long-term maintenance branch for 2.x customers",
				Author:      "kevin.duque",
				ReviewState: "APPROVED",
				CI:          &model.CIStatus{State: "SUCCESS", Workflow: "LTS Security Regression", Duration: "5m 20s"},
			},
			HasLocal:         false,
			Author:           "kevin.duque",
			AheadMain:        6,
			BehindMain:       8,
			UncommittedCount: 0,
			LastCommitMsg:    "lts: backport openssl vulnerability fix to 2.8 patchline",
			ChangedFiles:     []string{"crypto/openssl_compat.c"},
		},

		// 2. GELİŞTİRİCİ DALLARI VE PR'LARI (50 Developer Contributions)
		// Merged Pull Requests (Geçmiş başarılar)
		{
			ID:              "br-ent-merged-802",
			Name:            "feat/kafka-event-streaming",
			Hemisphere:      "upper",
			State:           "merged",
			StartSpineIndex: 7,
			EndSpineIndex:   9,
			BaseSHA:         "1808878",
			MergeSHA:        "3a0a69a",
			HasRemote:       true,
			RemoteRef:       "origin/feat/kafka-event-streaming",
			PR: &model.PRStatus{
				Number:      802,
				Title:       "feat(events): partitioned kafka message broker with zookeeper-less kraft",
				Author:      "alice.chen",
				ReviewState: "MERGED",
				CI:          &model.CIStatus{State: "SUCCESS", Workflow: "Broker Load Test", Duration: "2m 45s"},
			},
			HasLocal:      false,
			Author:        "alice.chen",
			LastCommitMsg: "kafka: add producer ack retry with exponential backoff",
			ChangedFiles:  []string{"services/events/kafka.go"},
		},
		{
			ID:              "br-ent-merged-807",
			Name:            "perf/graphql-query-batching",
			Hemisphere:      "upper",
			State:           "merged",
			StartSpineIndex: 9,
			EndSpineIndex:   11,
			BaseSHA:         "3a0a69a",
			MergeSHA:        "5c0c4bc",
			HasRemote:       true,
			RemoteRef:       "origin/perf/graphql-query-batching",
			PR: &model.PRStatus{
				Number:      807,
				Title:       "perf(graphql): dataloader N+1 batch caching across microservices",
				Author:      "lucas.muller",
				ReviewState: "MERGED",
				CI:          &model.CIStatus{State: "SUCCESS", Workflow: "GraphQL Benchmark", Duration: "1m 55s"},
			},
			HasLocal:      false,
			Author:        "lucas.muller",
			LastCommitMsg: "graphql: reduce redis roundtrips using bulk dataloader",
			ChangedFiles:  []string{"gateway/graphql/batcher.go"},
		},

		// Açık PR'lar (CI: RUNNING - Nabız animasyonlu)
		{
			ID:              "br-ent-open-815",
			Name:            "feat/grpc-service-mesh",
			Hemisphere:      "upper",
			State:           "open",
			StartSpineIndex: 11,
			EndSpineIndex:   14,
			BaseSHA:         "5c0c4bc",
			HasRemote:       true,
			RemoteRef:       "origin/feat/grpc-service-mesh",
			PR: &model.PRStatus{
				Number:      815,
				Title:       "feat(mesh): http/2 multiplexing and mTLS mutual authentication",
				Author:      "david.kim",
				ReviewState: "REVIEW_REQUIRED",
				CI: &model.CIStatus{
					State:    "RUNNING",
					Workflow: "mTLS Certificate Exchange Suite",
					Duration: "0m 42s",
				},
			},
			HasLocal:         true,
			LocalSHA:         "ea1109a",
			Author:           "david.kim",
			AheadMain:        3,
			BehindMain:       1,
			UncommittedCount: 0,
			LastCommitMsg:    "mesh: establish strict mTLS boundary on ingress",
			ChangedFiles:     []string{"services/mesh/tls.go", "deploy/helm/values.yaml"},
		},
		{
			ID:              "br-ent-open-818",
			Name:            "feat/billing-stripe-webhooks",
			Hemisphere:      "upper",
			State:           "open",
			StartSpineIndex: 12,
			EndSpineIndex:   15,
			BaseSHA:         "6d0d3cd",
			HasRemote:       true,
			RemoteRef:       "origin/feat/billing-stripe-webhooks",
			PR: &model.PRStatus{
				Number:      818,
				Title:       "feat(billing): idempotency keys for payment intent hooks",
				Author:      "priya.patel",
				ReviewState: "REVIEW_REQUIRED",
				CI: &model.CIStatus{
					State:    "RUNNING",
					Workflow: "Payment Gateway Simulation",
					Duration: "0m 22s",
				},
			},
			HasLocal:         false,
			Author:           "priya.patel",
			AheadMain:        2,
			BehindMain:       0,
			UncommittedCount: 0,
			LastCommitMsg:    "billing: lock transaction table during webhook replay",
			ChangedFiles:     []string{"services/billing/stripe.go"},
		},

		// Açık PR (CI: FAILURE - Kırmızı uyarı, log çıkarma ve panik analizi)
		{
			ID:              "br-ent-open-820",
			Name:            "fix/distributed-tracing-span",
			Hemisphere:      "upper",
			State:           "open",
			StartSpineIndex: 10,
			EndSpineIndex:   14,
			BaseSHA:         "4b0b5ab",
			HasRemote:       true,
			RemoteRef:       "origin/fix/distributed-tracing-span",
			PR: &model.PRStatus{
				Number:      820,
				Title:       "fix(telemetry): eliminate nil dereference in otel span export",
				Author:      "mehmet.yilmaz",
				ReviewState: "CHANGES_REQUESTED",
				CI: &model.CIStatus{
					State:      "FAILURE",
					Workflow:   "OpenTelemetry Integration CI",
					RunID:      "994182410",
					Duration:   "0m 58s",
					FailedStep: "Testing service mesh tracing spans",
					RawLog:     entCiFailLog,
				},
			},
			HasLocal:         true,
			LocalSHA:         "eb2210b",
			Author:           "mehmet.yilmaz",
			IsHEAD:           true,
			AheadMain:        4,
			BehindMain:       2,
			UncommittedCount: 3,
			LastCommitMsg:    "telemetry: check nil span context before flag evaluation",
			ChangedFiles:     []string{"services/telemetry/exporter.go", "services/telemetry/exporter_test.go"},
		},

		// Açık PR (ÇAKIŞMA VE CROSS-PR COLLISION: Auth ve Helm dosyaları)
		{
			ID:              "br-ent-open-825",
			Name:            "feat/auth-v2-jwt-rotation",
			Hemisphere:      "upper",
			State:           "open",
			StartSpineIndex: 11,
			EndSpineIndex:   15,
			BaseSHA:         "5c0c4bc",
			HasRemote:       true,
			RemoteRef:       "origin/feat/auth-v2-jwt-rotation",
			PR: &model.PRStatus{
				Number:      825,
				Title:       "feat(auth): dynamic asymmetric jwt rotation with jwks endpoint",
				Author:      "elena.rostova",
				ReviewState: "APPROVED",
				CI:          &model.CIStatus{State: "SUCCESS", Workflow: "Security & Auth Test", Duration: "1m 30s"},
			},
			HasLocal:         true,
			LocalSHA:         "ec3321c",
			Author:           "elena.rostova",
			AheadMain:        3,
			BehindMain:       1,
			UncommittedCount: 0,
			LastCommitMsg:    "auth: rotate public jwks keys every 6 hours",
			ChangedFiles:     []string{"services/auth/jwt.go", "api/v1/auth.go", "deploy/helm/values.yaml"},
			Conflict: &model.ConflictDetail{
				HasConflict:       true,
				Files:             []string{"deploy/helm/values.yaml"},
				IsChangelogOrLock: false,
				Strategy:          "merge-editor",
				PreviewUnified:    entConflictPreview,
			},
		},
		{
			ID:              "br-ent-open-828",
			Name:            "feat/oauth-pkce-flow",
			Hemisphere:      "upper",
			State:           "open",
			StartSpineIndex: 12,
			EndSpineIndex:   15,
			BaseSHA:         "6d0d3cd",
			HasRemote:       true,
			RemoteRef:       "origin/feat/oauth-pkce-flow",
			PR: &model.PRStatus{
				Number:      828,
				Title:       "feat(oauth): pkce challenge verification for native mobile apps",
				Author:      "carlos.silva",
				ReviewState: "REVIEW_REQUIRED",
				CI:          &model.CIStatus{State: "SUCCESS", Workflow: "OAuth Conformance Suite", Duration: "1m 18s"},
			},
			HasLocal:         false,
			Author:           "carlos.silva",
			AheadMain:        2,
			BehindMain:       0,
			UncommittedCount: 0,
			LastCommitMsg:    "oauth: verify sha256 code challenge on token exchange",
			ChangedFiles:     []string{"api/v1/auth.go", "services/auth/pkce.go"}, // Cross-PR collision with 825!
		},

		// Açık PR (Onaylı ve Hazır)
		{
			ID:              "br-ent-open-830",
			Name:            "fix/postgres-connection-leak",
			Hemisphere:      "upper",
			State:           "open",
			StartSpineIndex: 13,
			EndSpineIndex:   15,
			BaseSHA:         "7e0e2de",
			HasRemote:       true,
			RemoteRef:       "origin/fix/postgres-connection-leak",
			PR: &model.PRStatus{
				Number:      830,
				Title:       "fix(database): defer pgxpool.Conn.Release on error paths",
				Author:      "ayse.kaya",
				ReviewState: "APPROVED",
				CI:          &model.CIStatus{State: "SUCCESS", Workflow: "Postgres Stress Benchmark", Duration: "2m 10s"},
			},
			HasLocal:         false,
			Author:           "ayse.kaya",
			AheadMain:        1,
			BehindMain:       0,
			UncommittedCount: 0,
			LastCommitMsg:    "db: guarantee connection release in batch execution loops",
			ChangedFiles:     []string{"storage/postgres/pool.go"},
		},

		// 3. KALINTI VE YEREL DALLAR (Remnants, Fresh Local & WIP)
		// Squash Remnant / Local Remnant (origin: gone, aheadMain > 0)
		{
			ID:              "br-ent-remnant-gone",
			Name:            "feat/legacy-elasticsearch-drop",
			Hemisphere:      "upper",
			State:           "merged",
			StartSpineIndex: 8,
			EndSpineIndex:   11,
			BaseSHA:         "2909789",
			MergeSHA:        "5c0c4bc",
			HasRemote:       false,
			RemoteRef:       "origin/feat/legacy-elasticsearch-drop",
			OriginGone:      true,
			PR: &model.PRStatus{
				Number:      795,
				Title:       "refactor(search): migrate query cluster from es7 to opensearch",
				Author:      "marcus.vance",
				ReviewState: "MERGED",
				CI:          &model.CIStatus{State: "SUCCESS", Workflow: "Search Ingestion Tests", Duration: "3m 05s"},
			},
			HasLocal:      true,
			LocalSHA:      "5c0c4bc",
			Author:        "marcus.vance",
			AheadMain:     3,
			LastCommitMsg: "search: deprecate old elasticsearch index clients",
			ChangedFiles:  []string{"services/search/client.go"},
		},

		// Taze Yerel Dal (🌱 Yeni Yerel Dal, aheadMain == 0, remote yok)
		{
			ID:               "br-ent-fresh-local",
			Name:             "feat/edge-wasm-worker",
			Hemisphere:       "lower",
			State:            "open",
			StartSpineIndex:  15,
			EndSpineIndex:    16,
			BaseSHA:          "90100f0",
			HasRemote:        false,
			HasLocal:         true,
			LocalSHA:         "90100f0",
			Author:           "rachel.green",
			IsHEAD:           false,
			AheadMain:        0,
			BehindMain:       0,
			UncommittedCount: 0,
			LastCommitMsg:    "edge: bootstrap webassembly micro-worker scaffolding",
			ChangedFiles:     []string{"workers/wasm/worker.go"},
		},

		// Alt Ray Yerel WIP ve Deney Dalları (Lower Hemisphere)
		{
			ID:               "br-ent-local-ebpf",
			Name:             "wip/ebpf-tc-bandwidth-shaper",
			Hemisphere:       "lower",
			State:            "open",
			StartSpineIndex:  13,
			EndSpineIndex:    15,
			BaseSHA:          "7e0e2de",
			HasRemote:        false,
			HasLocal:         true,
			LocalSHA:         "ed4432d",
			Author:           "tarik.ozkan",
			IsHEAD:           false,
			AheadMain:        2,
			BehindMain:       0,
			UncommittedCount: 5,
			LastCommitMsg:    "wip(ebpf): attach tc ingress filter to veth pairs for rate limiting",
			ChangedFiles:     []string{"kernel/bpf/shaper.c", "kernel/bpf/loader.go"},
		},
		{
			ID:               "br-ent-local-wasm",
			Name:             "exp/v8-isolated-sandbox",
			Hemisphere:       "lower",
			State:            "open",
			StartSpineIndex:  14,
			EndSpineIndex:    16,
			BaseSHA:          "8f0f1ef",
			HasRemote:        false,
			HasLocal:         true,
			LocalSHA:         "ee5543e",
			Author:           "olga.ivanova",
			IsHEAD:           false,
			AheadMain:        1,
			BehindMain:       0,
			UncommittedCount: 2,
			LastCommitMsg:    "exp(sandbox): evaluate v8 isolate heap snapshot limits",
			ChangedFiles:     []string{"runtime/v8/isolate.cc"},
		},
	}

	// Geçmiş birleşmiş PR'lar: squash/eski merge nedeniyle merge noktaları omurga penceresinde
	// bulunamaz. Gerçek bir konumları olmadığı için parabol çizilmez, "geçmiş" çipinde listelenir.
	historyNames := []struct{ name, author string }{
		{"feat/audit-log-export", "alice.chen"}, {"fix/s3-multipart-retry", "david.kim"},
		{"chore/bump-protobuf-3-25", "elena.rostova"}, {"feat/rbac-service-accounts", "carlos.silva"},
		{"docs/runbook-failover", "sarah.jenkins"}, {"fix/grpc-keepalive-leak", "kenji.sato"},
		{"feat/tenant-quota-api", "fatima.zahra"}, {"refactor/config-loader", "lucas.muller"},
	}
	for i, h := range historyNames {
		num := 760 + i*3
		branches = append(branches, model.BranchArc{
			ID:              fmt.Sprintf("br-ent-history-%d", num),
			Name:            h.name,
			Hemisphere:      "upper",
			State:           "merged",
			StartSpineIndex: 0,
			EndSpineIndex:   len(nodes) - 1,
			MergeSHA:        "",
			OriginGone:      true,
			HistoryOnly:     true,
			Author:          h.author,
			LastCommitMsg:   "merged: " + h.name,
			PR: &model.PRStatus{
				Number:      num,
				Title:       "merged: " + h.name,
				Author:      h.author,
				ReviewState: "MERGED",
				CI:          &model.CIStatus{State: "SUCCESS", Workflow: "ci.yml", Duration: "—"},
			},
		})
	}

	return simWorkspace{
		repo: model.RepoMeta{
			ID:                "sim-enterprise",
			Name:              "MegaCorp Cloud Platform (50 Geliştirici)",
			Path:              "(Enterprise Çoklu Dal Simülasyonu)",
			DefaultBranch:     "main",
			RemoteName:        "origin",
			Ahead:             3,
			Behind:            0,
			DirtyCount:        3,
			Mode:              "simulation",
			VersionTag:        "v3.3.0-rc2",
			CurrentUser:       "mehmet.yilmaz",
			BranchProtected:   true,
			ProtectionSummary: "Korumalı (PR Zorunlu + 2 Onay + Strict CI + Force-Push Kilitli)",
			AppVersion:        AppVersion,
		},
		nodes:    nodes,
		branches: branches,
	}
}
