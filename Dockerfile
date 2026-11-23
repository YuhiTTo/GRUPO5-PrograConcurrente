
# Etapa 1: compilar los programas Go
FROM golang:1.27-alpine AS compilacion

WORKDIR /app

COPY . .

# Compilar coordinador, worker y API REST
RUN CGO_ENABLED=0 GOOS=linux go build -o /salida/coordinador ./cmd/coordinador && \
    CGO_ENABLED=0 GOOS=linux go build -o /salida/worker ./cmd/worker && \
    CGO_ENABLED=0 GOOS=linux go build -o /salida/api ./cmd/api

# Etapa 2: imagen de ejecución
FROM alpine:3.22

WORKDIR /app

# Copiar los ejecutables
COPY --from=compilacion /salida/coordinador ./coordinador
COPY --from=compilacion /salida/worker ./worker
COPY --from=compilacion /salida/api ./api

# Copiar modelo de referencia PC3
COPY modelos/ ./modelos/

# Comando predeterminado
CMD ["./coordinador"]
