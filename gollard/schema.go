package gollard

import gollardsql "github.com/yovico/gollard/sql"

type schemaScript struct {
	Version int64
	Script  []byte
}

func mallardSchemaScripts() []schemaScript {
	files := []struct {
		version int64
		path    string
	}{
		{0, "mallard/0000-setup.sql"},
		{1, "mallard/0001-applied-migrations.sql"},
	}
	out := make([]schemaScript, 0, len(files))
	for _, f := range files {
		data, err := gollardsql.MallardFS.ReadFile(f.path)
		if err != nil {
			panic("gollard: missing embedded schema file " + f.path + ": " + err.Error())
		}
		out = append(out, schemaScript{Version: f.version, Script: data})
	}
	return out
}
