package douban

type image struct {
	Large  string `json:"large"`
	Normal string `json:"normal"`
	Small  string `json:"small"`
}

type rating struct {
	Count int     `json:"count"`
	Max   int     `json:"max"`
	Value float64 `json:"value"`
}

type searchResponse struct {
	Count int          `json:"count"`
	Start int          `json:"start"`
	Total int          `json:"total"`
	Items []searchItem `json:"items"`
}

type searchItem struct {
	ID            string  `json:"id"`
	Type          string  `json:"type"`
	Title         string  `json:"title"`
	OriginalTitle string  `json:"original_title"`
	Year          string  `json:"year"`
	Abstract      string  `json:"abstract"`
	CardSubtitle  string  `json:"card_subtitle"`
	Pic           image   `json:"pic"`
	Rating        rating  `json:"rating"`
	URL           string  `json:"url"`
	URI           string  `json:"uri"`
	EpisodesInfo  string  `json:"episodes_info"`
	EpisodesCount int     `json:"episodes_count"`
	Target        *target `json:"target"`
}

type target struct {
	ID            string `json:"id"`
	Type          string `json:"type"`
	Title         string `json:"title"`
	OriginalTitle string `json:"original_title"`
	Year          string `json:"year"`
	Abstract      string `json:"abstract"`
	CardSubtitle  string `json:"card_subtitle"`
	Pic           image  `json:"pic"`
	URL           string `json:"url"`
	URI           string `json:"uri"`
}

type subjectDetailResponse struct {
	ID            string   `json:"id"`
	Type          string   `json:"type"`
	Title         string   `json:"title"`
	OriginalTitle string   `json:"original_title"`
	Intro         string   `json:"intro"`
	CardSubtitle  string   `json:"card_subtitle"`
	Year          string   `json:"year"`
	Genres        []string `json:"genres"`
	Countries     []string `json:"countries"`
	Languages     []string `json:"languages"`
	Pubdate       []string `json:"pubdate"`
	Durations     []string `json:"durations"`
	EpisodesCount int      `json:"episodes_count"`
	EpisodesInfo  string   `json:"episodes_info"`
	Rating        rating   `json:"rating"`
	Pic           image    `json:"pic"`
	IMDB          string   `json:"imdb"`
	IMDBID        string   `json:"imdb_id"`
	Directors     []string `json:"directors"`
	Actors        []string `json:"actors"`
}

type celebritiesResponse struct {
	Celebrities []celebrity `json:"celebrities"`
}

type celebrity struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	LatinName string `json:"latin_name"`
	Role      string `json:"role"`
	Type      string `json:"type"`
	Character string `json:"character"`
	Avatar    image  `json:"avatar"`
	CoverURL  string `json:"cover_url"`
	URL       string `json:"url"`
}

type photosResponse struct {
	Photos []photo `json:"photos"`
}

type photo struct {
	ID    string `json:"id"`
	Cover string `json:"cover"`
	Thumb string `json:"thumb"`
	Image image  `json:"image"`
}

type htmlDetail struct {
	ID            string
	Title         string
	OriginalTitle string
	Plot          string
	Rating        float64
	IMDBID        string
	Directors     []string
	Actors        []string
	Year          int
}
