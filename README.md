# 📡 kafka-broadcaster

Service that **consumes** JSON from a Kafka source topic (📥), **transforms** and **enriches** payloads (🔄), **routes** to target topics via headers or content rules (🧭), sends failures to a **DLQ** (⚠️), and can expose **Prometheus** metrics (📊).

A small **Kafka dev UI** (`kafkaui`) is included for local development: list topics, produce messages, and consume streams in the browser (🖥️).

## 🧰 Prerequisites

- **Go** — version in [`go.mod`](go.mod) (currently Go 1.25.x). Install from [https://go.dev/dl/](https://go.dev/dl/).
- **Kafka** — a reachable cluster (e.g. Docker 🐳, local install, or a shared dev broker). The consumer does not enable auto-creation of the **source** topic; that topic must exist before you start the broadcaster.
- **Make** (optional) — the [`Makefile`](Makefile) wraps build, run, and test targets.

## 1. 📦 Clone and fetch modules

```bash
git clone <repository-url>
cd kafka-broadcaster
go mod download
```

Or: `make tidy`

## 2. 🧱 Start Kafka and create topics

Use any Kafka setup you already have. The example config expects:

| Topic | Role |
|--------|------|
| `internal.events` | 📥 Source topic the broadcaster consumes |
| `broadcaster.dlq` | ⚠️ Dead-letter topic for failed produces (can often be auto-created on first produce if your cluster allows it) |
| One or more **target** topics | 🎯 Names you put in the `target-topic` header (or routes), e.g. `demo.out` |

Create the **source** topic before starting the app, for example with the Kafka distribution’s CLI (names and paths vary by install):

```bash
kafka-topics --bootstrap-server localhost:9092 --create --topic internal.events --partitions 1 --replication-factor 1
```

If you use a local broker on the default port, you can skip creating target/DLQ topics when the cluster allows automatic topic creation on produce (the service’s producer/DLQ clients request it).

## 3. ⚙️ Configuration

The broadcaster reads **`CONFIG_PATH`**. If unset, it defaults to `/etc/broadcaster/config.yaml` (typical in containers). For local runs, point it at a file under this repo.

Copy and edit the example:

```bash
cp config/config.example.yaml config/config.yaml
```

**Brokers:** In [`config/config.example.yaml`](config/config.example.yaml), `kafka.brokers` is set to `broker:9092`, which matches a Docker Compose/Kubernetes service hostname named `broker`. For Kafka on your machine, use something like:

```yaml
kafka:
  brokers:
    - "localhost:9092"
```

Adjust `source_topic`, `consumer_group`, `dlq_topic`, routing, transformation, and `metrics` as needed.

## 4. ▶️ Run the broadcaster

From the repository root:

```bash
make run
```

This runs `go run ./cmd/broadcaster` with `CONFIG_PATH` defaulting to `config/config.example.yaml` unless you override it:

```bash
CONFIG_PATH=config/config.yaml make run
```

Stop with `Ctrl+C` (⏹️).

If **metrics** are enabled in config (as in the example), Prometheus scrape endpoint: `http://localhost:9090/metrics` (path and port from your YAML) — 📊.

## 5. 🖥️ Run the dev UI

In a **second** terminal (with Kafka still reachable):

```bash
make run/ui
```

Defaults:

| Variable | Default | Purpose |
|----------|---------|---------|
| `KAFKA_BROKERS` | `localhost:9092` | Comma-separated seed brokers for the UI |
| `UI_PORT` | `8080` | HTTP listen port |

Examples:

```bash
KAFKA_BROKERS=localhost:9092 UI_PORT=8080 make run/ui
```

Open **http://localhost:8080** 🔗 in a browser. The UI talks to Kafka only; it does not start the broadcaster.

## 6. 🧪 Quick sanity check

1. With the broadcaster running and config pointing at your cluster, produce a JSON message to **`internal.events`** with a header **`target-topic`** set to a topic that exists or can be auto-created (see [`config/config.example.yaml`](config/config.example.yaml) for `routing.header_key` and transformation fields).
2. Confirm the broadcaster logs processing and that the message appears on the target topic (e.g. via the UI’s consume API or `kafka-console-consumer`).

## 🔨 Build binaries

```bash
make build      # ./bin/broadcaster
make build/ui   # ./bin/kafkaui
```

## 🛠️ Other Make targets

```bash
make help
```

Includes `test`, `test/all`, `lint`, `fmt`, `vet`, and `clean`.
