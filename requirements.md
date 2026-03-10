# Kafka Broadcaster Microservice

## Rationale

Within Global Commerce, many applications produce Kafka messages that are consumed by applications external to Global Commerce. All topics to which applications produce must be configured internally, which adds complexity.

The `kafka-broadcaster` microservice addresses this in three ways:

1. **Centralise configuration complexity** — Acts as the single point of configuration for all Kafka topic production, removing the burden from individual applications.

2. **Support data transformation** — Manipulates Kafka JSON payloads according to bespoke custom rules. Transformations include key renaming and changing key paths within the payload. Rules are defined in the topics configuration for the relevant client and look like a json mapping between keys in the input and output.

3. **Handle message routing** — Internal producer applications produce into a single external topic and attach Kafka headers to the message. The header value is used by `kafka-broadcaster` to resolve the target topic at runtime. For example, if the header is "string-value" then the target topic must be "string-value"

4. **Handle data enrichment** — Performs enrichments such as generating a unique ID from an incoming transaction ID using a defined algorithm. Algorithm must be configurable.

5. **Client-specific deployments** — To ensure best use of resources, each client application shall have its own deployment in the kubernetes cluster in a subnamespace

6. **Key metrics to be consumed in a grafana pod** — All key metrics (PCU USAGE, pod restart, number of failed messages and successful messages consumed, source and target topics and their respectice traffic) should be exposed in metrics private for consumption by Granfana

7. **Testing** — unit test, functional test, nft test, stubbed and e2e testing must be provided and integrated to the app build process 

8. **Make fike** — Use a make file to define the main operations (build, run, run the various tests suites as defined in point 7)

9. **Handling of failed messages** — If a message fails to be produced, it must be logged and send to a dedicated DLQ topic.