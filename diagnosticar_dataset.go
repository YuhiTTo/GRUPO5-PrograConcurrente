// Diagnostico exploratorio del dataset supervisado para PC3.
// Uso (desde la raiz del repositorio): go run diagnosticar_dataset.go
// Solo lee data/entrenamiento_base.csv; no modifica los datos.
package main

import (
    "bufio"
    "encoding/csv"
    "fmt"
    "io"
    "log"
    "os"
    "sort"
    "strconv"
    "strings"
    "time"
)

type Conteo struct { Total, Desfavorables int64 }
func (c *Conteo) agregar(y int) { c.Total++; if y == 1 { c.Desfavorables++ } }
func tasa(c Conteo) float64 { if c.Total==0 {return 0}; return float64(c.Desfavorables)*100/float64(c.Total) }

func indice(cab []string, nombre string) int {
    for i, x := range cab { if strings.TrimSpace(strings.TrimPrefix(x, "\uFEFF")) == nombre { return i } }
    log.Fatalf("No se encuentra columna %q",nombre)
    return -1
}
func año(fecha string) string {
    fecha = strings.TrimSpace(fecha)
    if len(fecha) < 4 {return "SIN_FECHA"}
    n,err := strconv.Atoi(fecha[:4])
    if err != nil || n < 1990 || n > 2100 {return "FORMATO_NO_RECONOCIDO"}
    return strconv.Itoa(n)
}
func categoria(s string) string {
    s = strings.TrimSpace(s)
    if s == "" {return "VACIO"}
    return s
}
func agregar(m map[string]*Conteo, k string, y int) {
    v, ok := m[k]; if !ok { v=&Conteo{}; m[k]=v }; v.agregar(y)
}
func mostrar(titulo string, m map[string]*Conteo, maximo int, porTotal bool) {
    type Fila struct { Nombre string; C Conteo }
    lista := make([]Fila,0,len(m))
    for k,v := range m { lista=append(lista,Fila{k,*v}) }
    sort.Slice(lista,func(i,j int)bool {
        if porTotal && lista[i].C.Total != lista[j].C.Total {return lista[i].C.Total > lista[j].C.Total}
        return lista[i].Nombre < lista[j].Nombre
    })
    if maximo > 0 && len(lista)>maximo {lista=lista[:maximo]}
    fmt.Printf("\n=== %s ===\n",titulo)
    fmt.Printf("%-34s %12s %12s %11s\n", "VALOR", "TOTAL", "CLASE 1", "TASA CL1")
    for _,r:=range lista {
        nombre := []rune(r.Nombre)
        if len(nombre)>34 {nombre=append(nombre[:31], '.', '.', '.')}
        fmt.Printf("%-34s %12d %12d %10.2f%%\n",string(nombre),r.C.Total,r.C.Desfavorables,tasa(r.C))
    }
    fmt.Printf("Valores diferentes: %d\n",len(m))
}
func main(){
    inicio:=time.Now()
    f,err:=os.Open("data/entrenamiento_base.csv");if err!=nil{log.Fatal(err)}
    defer f.Close()
    r:=csv.NewReader(bufio.NewReaderSize(f,1024*1024));r.ReuseRecord=true
    cab,err:=r.Read();if err!=nil{log.Fatal(err)}
    iRiesgo:=indice(cab,"riesgo")
    iMonto:=indice(cab,"monto_disponible")
    iMetodo:=indice(cab,"metodo")
    iCategoria:=indice(cab,"categoria")
    iFecha:=indice(cab,"fecha_publicacion")
    porAnio:=make(map[string]*Conteo)
    porMetodo:=make(map[string]*Conteo)
    porCategoria:=make(map[string]*Conteo)
    porMonto:=make(map[string]*Conteo)
    var total,errores,clase1 int64
    for {
        fila,e:=r.Read()
        if e==io.EOF {break}
        if e!=nil {log.Fatalf("Error CSV en fila %d: %v",total+errores+2,e)}
        y:=-1
        if fila[iRiesgo]=="0" {y=0} else if fila[iRiesgo]=="1" {y=1}
        if y==-1 {errores++;continue}
        total++; if y==1 {clase1++}
        agregar(porAnio,año(fila[iFecha]),y)
        agregar(porMetodo,categoria(fila[iMetodo]),y)
        agregar(porCategoria,categoria(fila[iCategoria]),y)
        if fila[iMonto]=="1" {agregar(porMonto,"MONTO DISPONIBLE",y)} else {agregar(porMonto,"MONTO FALTANTE/INVALIDO",y)}
    }
    fmt.Println("=== DIAGNOSTICO DE DATOS PARA PC3 ===")
    fmt.Printf("Registros validos: %d | Clase 0: %d | Clase 1: %d | Filas con etiqueta invalida: %d\n",total,total-clase1,clase1,errores)
    fmt.Printf("Tasa global clase 1: %.2f%%\n", tasa(Conteo{Total:total,Desfavorables:clase1}))
    mostrar("POR ANIO DE PUBLICACION",porAnio,0,false)
    mostrar("SEGUN DISPONIBILIDAD DE MONTO",porMonto,0,true)
    mostrar("TOP 15 METODOS",porMetodo,15,true)
    mostrar("TOP 15 CATEGORIAS",porCategoria,15,true)
    fmt.Println("\nTiempo de diagnostico:",time.Since(inicio))
    fmt.Println("Nota: las tasas son descriptivas; no demuestran causalidad ni prediccion anticipada.")
}
