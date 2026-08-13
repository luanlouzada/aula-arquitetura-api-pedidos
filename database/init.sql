CREATE TABLE IF NOT EXISTS orders (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    customer    TEXT        NOT NULL CHECK (btrim(customer) <> ''),
    -- LIMITE SAUDÁVEL: esta validação impede gravar um texto desconhecido como
    -- status, mesmo que algum erro tenha passado pelo código Go. É uma proteção extra.
    -- Mas ela só conhece valores isolados: não sabe que PAGO -> CANCELADO é proibido.
    -- A regra de mudança de estado ainda precisa existir no código de negócio.
    status      TEXT        NOT NULL CHECK (status IN ('PENDENTE', 'PAGO', 'CANCELADO')),
    total_cents BIGINT      NOT NULL CHECK (total_cents > 0),
    -- NO LEGADO, O MESMO DADO ATRAVESSA TODAS AS PARTES: version é armazenada
    -- nesta tabela, colocada na struct Order e enviada no JSON. Isso não é sempre
    -- errado, mas significa que mudar esse dado pode afetar banco, regra e API.
    version     INTEGER     NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS order_items (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    order_id         BIGINT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    product_name     TEXT   NOT NULL CHECK (btrim(product_name) <> ''),
    unit_price_cents BIGINT NOT NULL CHECK (unit_price_cents > 0),
    quantity         INTEGER NOT NULL CHECK (quantity > 0)
);

CREATE INDEX IF NOT EXISTS order_items_order_id_idx ON order_items(order_id);
