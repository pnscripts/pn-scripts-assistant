package tools

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeODS(t *testing.T, path string) {
	t.Helper()

	f, _ := os.Create(path)
	z := zip.NewWriter(f)
	w, _ := z.Create("content.xml")
	_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" xmlns:table="urn:oasis:names:tc:opendocument:xmlns:table:1.0" xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0"><office:body><office:spreadsheet>
<table:table table:name="Разходи">
<table:table-row><table:table-cell><text:p>Категория</text:p></table:table-cell><table:table-cell><text:p>Сума</text:p></table:table-cell></table:table-row>
<table:table-row table:number-rows-repeated="2"><table:table-cell><text:p>Храна</text:p></table:table-cell><table:table-cell office:value-type="float" office:value="12.5"><text:p>12,50 лв.</text:p></table:table-cell></table:table-row>
<table:table-row><table:table-cell><text:p>Наем</text:p></table:table-cell><table:table-cell office:value-type="float" office:value="400"><text:p>400,00 лв.</text:p></table:table-cell><table:table-cell table:number-columns-repeated="1000"/></table:table-row>
<table:table-row table:number-rows-repeated="1048000"><table:table-cell table:number-columns-repeated="1024"/></table:table-row>
</table:table></office:spreadsheet></office:body></office:document-content>`))
	z.Close()
	f.Close()
}

func TestTablesAreReadFromEveryFormat(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	csvPath := filepath.Join(dir, "разходи.csv")
	_ = os.WriteFile(csvPath, []byte("\ufeffКатегория;Сума\nХрана;\"12,50\"\nХрана;7,50\nОбщо;20,00\n"), 0o600)

	tabs, err := ReadTables(ctx, csvPath)
	if err != nil || len(tabs) != 1 || tabs[0].Header[0] != "Категория" || len(tabs[0].Rows) != 3 {
		t.Fatalf("csv: %v %+v", err, tabs)
	}

	odsPath := filepath.Join(dir, "budget.ods")
	writeODS(t, odsPath)

	tabs, err = ReadTables(ctx, odsPath)
	if err != nil || len(tabs) != 1 || tabs[0].Name != "Разходи" || len(tabs[0].Rows) != 3 || tabs[0].Rows[2][1] != "400" {
		t.Fatalf("ods: %v %+v", err, tabs)
	}

	book, err := BuildWorkbook([]Sheet{
		{Title: "January", Rows: [][]string{{"Item", "Cost"}, {"Tea", "3.5"}, {"Milk", "2"}}},
		{Title: "February", Rows: [][]string{{"Item", "Cost"}, {"Bread", "1.2"}}},
	})
	if err != nil {
		t.Fatal(err)
	}

	xlsxPath := filepath.Join(dir, "two.xlsx")
	_ = os.WriteFile(xlsxPath, book, 0o600)

	tabs, err = ReadTables(ctx, xlsxPath)
	if err != nil || len(tabs) != 2 || tabs[1].Name != "February" || tabs[0].Rows[0][1] != "3.5" {
		t.Fatalf("xlsx: %v %+v", err, tabs)
	}

	if _, err := ReadTables(ctx, "relative.csv"); err == nil {
		t.Error("a relative path was read")
	}

	if _, err := ReadTables(ctx, filepath.Join(dir, "x.pdf")); err == nil {
		t.Error("a missing pdf was read")
	}
}

func TestATableQuestionBecomesACheckedCalculation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sales.csv")
	_ = os.WriteFile(path, []byte("Region,Sales\nNorth,100\nSouth,250.50\nNorth,49.50\nTotal,400\n"), 0o600)

	var asked string

	tables := Tables{Ask: func(_ context.Context, system, user string) (string, string, error) {
		asked = system + "\n" + user

		return `{"group_by":["Region"],"aggregate":[{"column":"Sales","fn":"sum","as":"Total sales"}],"sort":{"column":"Total sales","desc":true},"explain":"Sales by region."}`, "m", nil
	}}

	c, err := tables.Calculate(context.Background(), path, "", "sales by region", nil)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(asked, `"Sales": number`) || !strings.Contains(asked, "never put a result") {
		t.Errorf("the model was not told the columns or the rule: %s", asked)
	}

	if strings.Join(c.Rows[0], "|") != "South|250.5" || strings.Join(c.Rows[1], "|") != "North|149.5" || c.Model != "m" {
		t.Errorf("%+v", c.Rows)
	}

	out, err := CalculateTable{With: tables}.Execute(context.Background(),
		json.RawMessage(`{"path":"`+path+`","question":"sales by region"}`))
	if err != nil || !strings.Contains(out, "| South | 250.5 |") || !strings.Contains(out, "OK: Total sales: the groups add up to 400") ||
		!strings.Contains(out, "rows 5 are the sheet's own totals") {
		t.Errorf("%v\n%s", err, out)
	}

	book, err := Workbook(c)
	if err != nil {
		t.Fatal(err)
	}

	_ = os.WriteFile(filepath.Join(dir, "out.xlsx"), book, 0o600)

	back, _ := ReadTables(context.Background(), filepath.Join(dir, "out.xlsx"))
	if len(back) != 2 || back[0].Name != "Result" || back[1].Name != "Checks" {
		t.Errorf("%+v", back)
	}

	if _, err := (Tables{}).Calculate(context.Background(), path, "", "sales by region", nil); !errors.Is(err, ErrNoTableModel) {
		t.Errorf("%v", err)
	}

	sum, err := TableSummary{}.Execute(context.Background(), json.RawMessage(`{"path":"`+path+`"}`))
	if err != nil || !strings.Contains(sum, "Sales: numbers, 3 filled; sum 400") || !strings.Contains(sum, "the sheet's total 400 matches") {
		t.Errorf("%v\n%s", err, sum)
	}

	if _, err := (TableSummary{}).Execute(context.Background(), json.RawMessage(`{"path":"`+path+`","sheet":"Nope"}`)); err == nil {
		t.Error("a missing sheet was found")
	}
}
