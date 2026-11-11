CREATE TABLE vessels (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    capacity INTEGER NOT NULL CHECK (capacity > 0),
    current_location TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE cargoes (
    id UUID PRIMARY KEY,
    code TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    weight_kg DOUBLE PRECISION NOT NULL CHECK (weight_kg > 0),
    status TEXT NOT NULL CHECK (status IN ('pending', 'in_transit', 'delivered')),
    vessel_id UUID NOT NULL REFERENCES vessels(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_cargoes_vessel_id ON cargoes(vessel_id);
CREATE INDEX idx_cargoes_status ON cargoes(status);
CREATE INDEX idx_cargoes_created_at ON cargoes(created_at DESC);

CREATE TABLE tracking (
    id UUID PRIMARY KEY,
    cargo_id UUID NOT NULL REFERENCES cargoes(id) ON DELETE RESTRICT,
    location TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'in_transit', 'delivered')),
    occurred_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_tracking_cargo_occurred ON tracking(cargo_id, occurred_at DESC);

CREATE TABLE cargo_events (
    id UUID PRIMARY KEY,
    event_id UUID NOT NULL UNIQUE,
    cargo_id UUID NOT NULL REFERENCES cargoes(id) ON DELETE RESTRICT,
    old_status TEXT,
    new_status TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    received_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_cargo_events_cargo_occurred ON cargo_events(cargo_id, occurred_at DESC);
