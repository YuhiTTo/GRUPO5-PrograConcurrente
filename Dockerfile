
# Etapa 1: compilar los programas Go
FROM golang:1.27-alpine AS compilacion

WORKDIR /app

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /salida/coordinador ./cmd/coordinador && \
    CGO_ENABLED=0 GOOS=linux go build -o /salida/worker ./cmd/worker

# Etapa 2: imagen de ejecución
FROM alpine:3.22

WORKDIR /app

# Copiar ejecutables compilados
COPY --from=compilacion /salida/coordinador ./coordinador
COPY --from=compilacion /salida/worker ./worker

# Copiar configuración del modelo PC3
COPY modelos/ ./modelos/

# Comando predeterminado
CMD ["./coordinador"]
