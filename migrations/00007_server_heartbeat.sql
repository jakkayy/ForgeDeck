-- +goose Up
-- Latest Agent-reported status. These are operational hints, not trusted data.
ALTER TABLE servers
    ADD COLUMN docker_available boolean NOT NULL DEFAULT false,
    ADD COLUMN docker_version   text,
    ADD COLUMN cpu_percent      real CHECK (cpu_percent BETWEEN 0 AND 100),
    ADD COLUMN memory_percent   real CHECK (memory_percent BETWEEN 0 AND 100),
    ADD COLUMN disk_percent     real CHECK (disk_percent BETWEEN 0 AND 100);

-- +goose Down
ALTER TABLE servers
    DROP COLUMN docker_available,
    DROP COLUMN docker_version,
    DROP COLUMN cpu_percent,
    DROP COLUMN memory_percent,
    DROP COLUMN disk_percent;
