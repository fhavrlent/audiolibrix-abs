# audiolibrix-abs

[Audiobookshelf](https://www.audiobookshelf.org/) custom metadata provider for [Audiolibrix](https://www.audiolibrix.com/cs/).
Inspired by [audioteka-abs](https://github.com/lakafior/audioteka-abs).

## Fetched metadata

- Title and subtitle
- Author(s) and narrator(s)
- Publisher and year of publication
- Description
- Cover (1000 px)
- Genres
- Series with book number (e.g. *Zaklínač #2*)
- Language
- Duration
- Tag `zkrácená verze` for abridged editions

## Running

Requires Docker with Docker Compose. Images for `linux/amd64` and `linux/arm64` are published
to `ghcr.io/fhavrlent/audiolibrix-abs`.

Create a `compose.yml` (or copy the one from this repository):

```yaml
services:
  audiolibrix-abs:
    image: ghcr.io/fhavrlent/audiolibrix-abs:latest
    container_name: audiolibrix-abs
    environment:
      - LANGUAGES=cs,sk
      - ADD_LINK_TO_DESCRIPTION=true
    restart: unless-stopped
    ports:
      - "3001:3001"
```

```sh
docker compose up -d                      # start
docker compose pull && docker compose up -d  # update
docker compose logs -f                    # logs
```

To pin a version instead of `latest`, use e.g. `ghcr.io/fhavrlent/audiolibrix-abs:1.0.0` (or `:1.0`, `:1`).

### Environment variables

| Variable | Default | Description |
|---|---|---|
| `LANGUAGES` | `cs,sk` | Comma-separated book languages to return (`cs`, `sk`, `en`, `de`, `fr`, `it`, `ru`, `es`, `ua`), or `all`. Audiolibrix search results mix languages, so this filters on the language shown on each book's page. |
| `ADD_LINK_TO_DESCRIPTION` | `true` | Prepend a link to the Audiolibrix page to the description. |
| `MAX_RESULTS` | `10` | Maximum number of matches returned to Audiobookshelf. |
| `MAX_FETCH` | `30` | Maximum number of book pages fetched per search. |
| `CONCURRENCY` | `5` | Book pages fetched in parallel. Lower it to be gentler on the site. |
| `AUTH_TOKEN` | *(empty)* | If set, Audiobookshelf must send exactly this value. If empty, any non-empty value is accepted. |
| `PORT` | `3001` | Port to listen on. |

## Adding to Audiobookshelf

1. Settings → Item Metadata Utils → Custom Metadata Providers → Add
2. Name: `Audiolibrix`
3. URL: `http://<your-ip>:3001`
4. Authorization Header Value: any non-empty value (or your `AUTH_TOKEN`)
5. Save, then choose *Audiolibrix* as the provider when matching a book.

### Books missing from search

Audiolibrix leaves books that are no longer sold out of its search, but their pages usually still
work. To match such a book, put its Audiolibrix link (e.g.
`https://www.audiolibrix.com/cs/Directory/Book/596/Audiokniha-Druhy-gong-Agatha-Christie`) or
`audiolibrix:596` in the search box. The provider then reads that page directly and skips the
`LANGUAGES` filter.

## How it works

The provider runs Audiolibrix's regular search (`/cs/Search/Results`), ranks the results
against the title and author sent by Audiobookshelf (the site search is loose), and fetches
book pages in ranked order until `MAX_RESULTS` books in an allowed language are found.
It always reads the Czech version of the site, because the parser relies on its field labels.

## Development

```sh
go test ./...
PORT=3001 go run .
curl -H 'Authorization: x' 'localhost:3001/search?query=hobit&author=tolkien'
```

Build the image locally: `docker build -t audiolibrix-abs .`

Tests run against pages in `testdata/` that copy only the markup the parser reads from
Audiolibrix (class names and field labels), filled with made-up books. When Audiolibrix changes
its HTML, update them and adjust the parser in `audiolibrix.go`.

## Releasing

Pushing a version tag runs the tests and publishes the image:

```sh
git tag v1.0.0
git push origin v1.0.0
```

`v1.2.3` publishes the tags `1.2.3`, `1.2`, `1` and `latest`. Pre-release tags such as
`v1.2.3-rc.1` do not move `latest`.
