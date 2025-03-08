FROM alpine:latest

COPY translator /app/translator
COPY configs/default.yaml /app/config.yaml
COPY README.md /app/README.md

WORKDIR /app

ENTRYPOINT ["/app/translator"]