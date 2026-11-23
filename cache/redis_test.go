package cache

import (
	"context"
	"testing"
	"time"
)

func TestClienteNilEsSeguro(t *testing.T) {
	var c *Cliente

	if _, ok := c.Obtener(context.Background(), "k"); ok {
		t.Fatal("un cliente nil nunca debe producir un acierto")
	}
	c.Guardar(context.Background(), "k", []byte("v"), time.Second)
	if err := c.Cerrar(); err != nil {
		t.Fatalf("Cerrar sobre nil no debe fallar: %v", err)
	}
}

func TestClavePrediccionEsEstableYDistingueCampos(t *testing.T) {
	a := ClavePrediccion("OPEN", "SERVICIOS", "2025-06-15")
	b := ClavePrediccion("OPEN", "SERVICIOS", "2025-06-15")
	c := ClavePrediccion("OPEN", "SERVICIOS2025-06-15", "")

	if a != b {
		t.Fatal("la misma entrada debe dar la misma clave")
	}
	if a == c {
		t.Fatal("el separador debe evitar colisiones por concatenación")
	}
}

func TestConectarFallaSinServidor(t *testing.T) {
	t.Setenv("REDIS_ADDR", "127.0.0.1:1")
	ctx, cancelar := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelar()

	if _, err := Conectar(ctx); err == nil {
		t.Fatal("se esperaba error sin servidor Redis")
	}
}
