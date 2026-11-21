package datos

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"testing"
)

func TestCargarEntrenamientoCSV(t *testing.T) {
	ruta := filepath.Join(
		t.TempDir(),
		"entrenamiento.csv",
	)

	archivo, err := os.Create(ruta)
	if err != nil {
		t.Fatal(err)
	}

	writer := csv.NewWriter(archivo)

	filas := [][]string{
		{
			"ocid", "metodo", "categoria",
			"fecha_publicacion", "buyer_id",
			"metodo_detalle", "titulo",
			"descripcion", "riesgo",
		},
		{
			"oc1", "", "goods", "2018-01-01",
			"buyer1", "LICITACION PUBLICA",
			"COMPRA DE COMPUTADOR",
			"Descripcion de prueba", "0",
		},
		{
			"oc2", "open", "services", "2023-05-15",
			"buyer2", "CONCURSO PUBLICO",
			"SERVICIO DE LIMPIEZA", "", "1",
		},
		{
			"oc3", "open", "goods", "2024-06-10",
			"buyer3", "LICITACION",
			"COMPRA", "Descripcion", "0",
		},
		{
			"oc4", "open", "goods", "2020-07-20",
			"buyer4", "LICITACION",
			"COMPRA", "Descripcion", "2",
		},
	}

	if err := writer.WriteAll(filas); err != nil {
		t.Fatal(err)
	}

	writer.Flush()

	if err := archivo.Close(); err != nil {
		t.Fatal(err)
	}

	datos, stats, err := CargarEntrenamientoCSV(
		ruta,
		2018,
		2023,
		true,
	)

	if err != nil {
		t.Fatal(err)
	}

	if len(datos) != 2 {
		t.Fatalf("esperados 2 registros, recibidos %d", len(datos))
	}

	if stats.Clase0 != 1 || stats.Clase1 != 1 {
		t.Fatalf("distribucion incorrecta: %+v", stats)
	}

	if stats.FueraPeriodo != 1 {
		t.Fatalf("esperado 1 registro fuera del periodo")
	}

	if stats.Invalidos != 1 {
		t.Fatalf("esperado 1 registro invalido")
	}

	if stats.MetodoVacio != 1 {
		t.Fatalf("no se conservo el metodo vacio")
	}

	if stats.DescripcionVacia != 1 {
		t.Fatalf("no se detecto la descripcion vacia")
	}

	if datos[0].Clase != 0 || datos[1].Clase != 1 {
		t.Fatal("etiquetas incorrectas")
	}

	if datos[0].NTokensDesc == 0 {
		t.Fatal("no se codifico la descripcion")
	}

	if datos[1].NTokensDesc != 0 {
		t.Fatal("la descripcion vacia debe generar cero tokens")
	}
}
