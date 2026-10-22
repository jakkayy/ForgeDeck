-- Benchmark seed (docs/12-measurements.md): run on a fresh database after create-user --email bench@forgedeck.local.

BEGIN;

CREATE TEMP TABLE bench_owner AS SELECT id FROM users WHERE email = 'bench@forgedeck.local';
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM bench_owner) THEN
        RAISE EXCEPTION 'create the user bench@forgedeck.local first';
    END IF;
END $$;

INSERT INTO projects (id, name, slug, created_by)
SELECT ('00000000-0000-4000-8000-0000000000' || lpad(p::text, 2, '0'))::uuid,
       'Project ' || lpad(p::text, 2, '0'), 'project-' || lpad(p::text, 2, '0'), (SELECT id FROM bench_owner)
FROM generate_series(1, 10) p;

INSERT INTO project_members (project_id, user_id, role)
SELECT id, (SELECT id FROM bench_owner), 'owner' FROM projects;

-- The ciphertexts are placeholders: nothing in the benchmark decrypts them.
INSERT INTO git_connections (id, project_id, provider, display_name, base_url, token_ciphertext, token_key_id, webhook_ciphertext, webhook_key_id, created_by)
SELECT ('00000000-0000-4000-8001-0000000000' || lpad(row_number() OVER (ORDER BY name)::text, 2, '0'))::uuid,
       id, 'gitlab', 'GitLab', 'https://gitlab.example.com', '\x00', 'v1', '\x00', 'v1', (SELECT id FROM bench_owner)
FROM projects;

INSERT INTO servers (id, project_id, name, last_seen_at, agent_version, docker_available, docker_version, cpu_percent, memory_percent, disk_percent)
SELECT ('00000000-0000-4000-8002-' || lpad(n::text, 12, '0'))::uuid,
       ('00000000-0000-4000-8000-0000000000' || lpad(((n - 1) / 5 + 1)::text, 2, '0'))::uuid,
       'server-' || lpad(n::text, 3, '0'),
       CASE WHEN n % 5 = 0 THEN now() - interval '1 hour' ELSE now() END,
       '0.1.0', true, '27.3.1', (n * 7) % 100, (n * 11) % 100, (n * 13) % 100
FROM generate_series(1, 50) n;

INSERT INTO agents (server_id, credential_hash)
SELECT id, sha256(('fda_bench_' || lpad(row_number() OVER (ORDER BY name)::text, 3, '0'))::bytea) FROM servers;

INSERT INTO applications (id, project_id, name, git_connection_id, repository_id, repository_full_name, branch, dockerfile_path, build_context, created_by)
SELECT ('00000000-0000-4000-8003-' || lpad(n::text, 12, '0'))::uuid,
       ('00000000-0000-4000-8000-0000000000' || lpad(((n - 1) / 10 + 1)::text, 2, '0'))::uuid,
       'app-' || lpad(n::text, 3, '0'),
       ('00000000-0000-4000-8001-0000000000' || lpad(((n - 1) / 10 + 1)::text, 2, '0'))::uuid,
       n::text, 'acme/app-' || lpad(n::text, 3, '0'), 'main', 'Dockerfile', '.', (SELECT id FROM bench_owner)
FROM generate_series(1, 100) n;

INSERT INTO environments (id, application_id, name, server_id, auto_deploy, config, published_port, created_by)
SELECT ('00000000-0000-4000-8004-' || lpad(n::text, 12, '0'))::uuid,
       a.id, CASE WHEN n % 2 = 0 THEN 'production' ELSE 'staging' END,
       ('00000000-0000-4000-8002-' || lpad((((a.ord - 1) / 10) * 5 + 1 + (n % 5))::text, 12, '0'))::uuid,
       n % 2 = 1,
       '{"container_port":8080,"published_port":null,"bind_address":"127.0.0.1","env":{},"secrets":[],"volumes":[],"restart":"unless-stopped","health_check":{"type":"http","path":"/healthz","timeout_seconds":5,"interval_seconds":5}}'::jsonb,
       NULL, (SELECT id FROM bench_owner)
FROM (SELECT id, row_number() OVER (ORDER BY name) AS ord FROM applications) a
JOIN LATERAL generate_series((a.ord - 1) * 2 + 1, a.ord * 2) n ON true;

INSERT INTO jobs (id, project_id, type, worker_kind, status, payload, result, application_id, attempt_count, created_at, started_at, finished_at)
SELECT ('00000000-0000-4000-8005-' || lpad(n::text, 12, '0'))::uuid,
       a.project_id, 'build', 'runner',
       CASE WHEN n % 11 = 0 THEN 'failed' ELSE 'succeeded' END,
       jsonb_build_object('application_id', a.id, 'repository_full_name', a.repository_full_name, 'branch', 'main',
                          'commit_sha', substr(md5(n::text) || md5((n + 1)::text), 1, 40), 'commit_message', 'Change ' || n),
       CASE WHEN n % 11 = 0 THEN NULL ELSE jsonb_build_object('commit_sha', substr(md5(n::text) || md5((n + 1)::text), 1, 40),
            'image', 'ghcr.io/acme/' || a.name, 'tag', 'abc1234', 'digest', 'sha256:' || md5(n::text) || md5((n + 1)::text),
            'image_ref', 'ghcr.io/acme/' || a.name || '@sha256:' || md5(n::text) || md5((n + 1)::text)) END,
       a.id, 1,
       now() - (interval '90 days') * (1 - n / 20000.0),
       now() - (interval '90 days') * (1 - n / 20000.0) + interval '1 second',
       now() - (interval '90 days') * (1 - n / 20000.0) + interval '1 minute'
FROM generate_series(1, 20000) n
JOIN (SELECT id, project_id, name, repository_full_name, row_number() OVER (ORDER BY name) AS ord FROM applications) a
  ON a.ord = (n % 100) + 1;

UPDATE jobs SET error_code = 'build_failed', error_message = 'The build failed.' WHERE status = 'failed';

INSERT INTO job_log_chunks (job_id, attempt_number, sequence_number, stream, content)
SELECT j.id, 1, s, CASE WHEN s % 40 = 0 THEN 'stderr' ELSE 'stdout' END,
       'Step ' || s || '/300 : RUN make target-' || s || ' && echo done'
FROM (SELECT id FROM jobs ORDER BY created_at DESC LIMIT 1000) j, generate_series(1, 300) s;

INSERT INTO deployments (id, environment_id, source_build_job_id, commit_sha, image_ref, config, status, error_code, created_by, created_at, started_at, finished_at)
SELECT ('00000000-0000-4000-8006-' || lpad(n::text, 12, '0'))::uuid,
       e.id, j.id, j.payload->>'commit_sha',
       COALESCE(j.result->>'image_ref', 'ghcr.io/acme/x@sha256:' || md5(n::text) || md5((n + 1)::text)),
       e.config,
       CASE WHEN n % 8 = 0 THEN 'failed' ELSE 'succeeded' END,
       CASE WHEN n % 8 = 0 THEN 'health_check_failed' END,
       (SELECT id FROM bench_owner),
       now() - (interval '60 days') * (1 - (n % 40) / 40.0),
       now() - (interval '60 days') * (1 - (n % 40) / 40.0),
       now() - (interval '60 days') * (1 - (n % 40) / 40.0) + interval '40 seconds'
FROM generate_series(1, 8000) n
JOIN (SELECT id, config, row_number() OVER (ORDER BY id) AS ord FROM environments) e ON e.ord = ((n - 1) / 40) + 1
JOIN jobs j ON j.id = ('00000000-0000-4000-8005-' || lpad(((n % 20000) + 1)::text, 12, '0'))::uuid;

INSERT INTO releases (application_id, environment_id, deployment_id, commit_sha, image_ref, config, created_at)
SELECT e.application_id, d.environment_id, d.id, d.commit_sha, d.image_ref, d.config, d.finished_at
FROM deployments d JOIN environments e ON e.id = d.environment_id
WHERE d.status = 'succeeded';

INSERT INTO audit_logs (project_id, actor_user_id, actor, action, target_type, target_id, target_name, metadata, created_at)
SELECT p.id, (SELECT id FROM bench_owner), 'bench@forgedeck.local',
       (ARRAY['environment.deploy', 'application.create', 'secret.set', 'member.add', 'build.start', 'server.rotate_credential'])[1 + n % 6],
       'application', md5(n::text), 'app-' || lpad((n % 100 + 1)::text, 3, '0'), '{}'::jsonb,
       now() - (interval '200 days') * (1 - n / 50000.0)
FROM generate_series(1, 50000) n
JOIN (SELECT id, row_number() OVER (ORDER BY name) AS ord FROM projects) p ON p.ord = (n % 10) + 1;

COMMIT;
ANALYZE;
