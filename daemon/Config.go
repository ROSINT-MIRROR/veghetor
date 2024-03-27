package daemon

type Config struct {
	DatabaseType   string `default:"sqlite3" usage:"Database type (sqlite3)"`
	DatabasePath   string `default:"./veghetor.db" usage:"Database path"`
	ListenPath     string `default:":8080" usage:"Listen path"`
	UpdateInterval int    `default:"1800" usage:"Update interval in seconds"`
	InitialLaunch  bool   `default:"false" usage:"Initial launch"`
}
