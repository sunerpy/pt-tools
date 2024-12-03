package qbit

type QbitTorrentProperties struct {
	SavePath               string  `json:"save_path"`
	CreationDate           int64   `json:"creation_date"`
	PieceSize              int64   `json:"piece_size"`
	Comment                string  `json:"comment"`
	TotalWasted            int64   `json:"total_wasted"`
	TotalUploaded          int64   `json:"total_uploaded"`
	TotalUploadedSession   int64   `json:"total_uploaded_session"`
	TotalDownloaded        int64   `json:"total_downloaded"`
	TotalDownloadedSession int64   `json:"total_downloaded_session"`
	UploadLimit            int64   `json:"up_limit"`
	DownloadLimit          int64   `json:"dl_limit"`
	TimeElapsed            int64   `json:"time_elapsed"`
	SeedingTime            int64   `json:"seeding_time"`
	NumConnections         int     `json:"nb_connections"`
	NumConnectionsLimit    int     `json:"nb_connections_limit"`
	ShareRatio             float64 `json:"share_ratio"`
	AdditionDate           int64   `json:"addition_date"`
	CompletionDate         int64   `json:"completion_date"`
	CreatedBy              string  `json:"created_by"`
	AverageDownloadSpeed   int64   `json:"dl_speed_avg"`
	DownloadSpeed          int64   `json:"dl_speed"`
	ETA                    int64   `json:"eta"`
	LastSeen               int64   `json:"last_seen"`
	Peers                  int     `json:"peers"`
	PeersTotal             int     `json:"peers_total"`
	PiecesHave             int     `json:"pieces_have"`
	PiecesNum              int     `json:"pieces_num"`
	Reannounce             int64   `json:"reannounce"`
	Seeds                  int     `json:"seeds"`
	SeedsTotal             int     `json:"seeds_total"`
	TotalSize              int64   `json:"total_size"`
	AverageUploadSpeed     int64   `json:"up_speed_avg"`
	UploadSpeed            int64   `json:"up_speed"`
	IsPrivate              bool    `json:"isPrivate"`
}
