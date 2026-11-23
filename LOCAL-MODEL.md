# Local model: classify music search strings with `fm`

This document tells how to use the `fm` command-line tool (Apple Foundation Models CLI, `/usr/bin/fm`) as a classifier for music search strings. The model runs on the device. No data goes to the cloud.

The classifier gives this information for a search string:

- The probable full title, for example "The Dark Side of the Moon" for "dark side moon".
- The category: `track`, `artist`, `album` or `radio_station`.
- A confidence: `high`, `medium` or `low`.

All results in this document come from tests on macOS 27 (2026-11-22). The samples are small (10 to 18 search strings). Use them as a guide, not as a measurement.

## Quick start

```bash
fm respond --no-stream -g \
  -i 'Identify what this music search string refers to.' \
  --schema enum.json \
  'Search string: "sinatra"'
```

Output:

```json
{"full_title": "Frank Sinatra", "category": "artist", "confidence": "high"}
```

## Commands and options

`fm respond` gives a response to a prompt. These options are important for a classifier:

| Option | Function |
|---|---|
| `--schema <file or JSON>` | Makes the model write JSON that agrees with the schema. |
| `-i`, `--instructions <text>` | Gives the instructions to the model. |
| `--no-stream` | Writes the full result at one time. Streaming is on by default. |
| `-g`, `--greedy` | Uses greedy sampling. The same input gives the same output. |
| `--use-case <case>` | `general` (default) or `content-tagging`. Do not use `content-tagging` here (see below). |

`fm schema object` makes a schema from command-line options. It cannot make an `enum`, thus it is not sufficient for a classifier (see below).

`fm serve` starts a local server with a Chat Completions API. It is not faster than `fm respond`, and it gave worse answers (see "fm serve compared with fm respond" below).

## Schema rules for `fm`

`fm` uses JSON Schema, but it has two more requirements. A schema without them fails with this error:

```
Error: Invalid schema at 'schema.json': The data couldn't be read because it is missing.
```

The root object must have:

- `title`: the name of the type.
- `x-order`: a list of the property names, in the order that the model writes them.

### Example: a schema that fails

This schema has no `title` and no `x-order`, thus `fm` does not accept it:

```json
{
  "type": "object",
  "properties": {
    "category": {
      "type": "string",
      "enum": ["track", "artist", "album", "radio_station"],
      "description": "The most likely music entity type for the input string."
    },
    "confidence": {
      "type": "string",
      "enum": ["high", "medium", "low"]
    }
  },
  "required": ["category", "confidence"]
}
```

### Example: a schema from `fm schema object`

```bash
fm schema object --name MusicClassification \
  --string category --description 'One of: track, artist, album, radio_station' \
  --string confidence --description 'One of: high, medium, low'
```

Output:

```json
{
  "required": ["category", "confidence"],
  "additionalProperties": false,
  "x-order": ["category", "confidence"],
  "title": "MusicClassification",
  "properties": {
    "category": {
      "description": "One of: track, artist, album, radio_station",
      "type": "string"
    },
    "confidence": {
      "type": "string",
      "description": "One of: high, medium, low"
    }
  },
  "type": "object"
}
```

`fm` accepts this schema, but the fields are free text. In a test, the model wrote `")"` as the confidence. For a classifier, write the schema manually and use `enum`.

## Schema 1: category only

This schema adds `title`, `x-order` and `enum` values:

```json
{
  "title": "MusicClassification",
  "type": "object",
  "additionalProperties": false,
  "x-order": ["category", "confidence"],
  "required": ["category", "confidence"],
  "properties": {
    "category": {
      "type": "string",
      "enum": ["track", "artist", "album", "radio_station"],
      "description": "The most likely music entity type for the search string."
    },
    "confidence": {
      "type": "string",
      "enum": ["high", "medium", "low"]
    }
  }
}
```

### Instructions and use case

We tested the instructions and the use case on 10 search strings:

| Instructions | Use case | Correct |
|---|---|---|
| "Classify the music search string as a track, artist, album or radio station." | content-tagging | 10/10 |
| "Classify the music search string as a track, artist, album or radio station." | general | 10/10 |
| "Classify this music search string." | general | 10/10 |
| No instructions | general | 9/10 |
| "Classify this music search string." | content-tagging | 6/10 |
| No instructions | content-tagging | 3/10 |

With the `general` use case, the enum values and the field description give sufficient information. The `content-tagging` use case works only when the instructions list the categories.

Example:

```bash
fm respond --no-stream -g -i 'Classify this music search string.' \
  --schema schema1.json 'Abbey Road'
# {"confidence": "high", "category": "album"}
```

## Schema 2: full title, category and confidence

This schema adds `full_title`. It is the current `enum.json`:

```json
{
  "title": "MusicClassification",
  "type": "object",
  "additionalProperties": false,
  "x-order": ["full_title", "category", "confidence"],
  "required": ["full_title", "category", "confidence"],
  "properties": {
    "full_title": {
      "type": "string",
      "description": "The full official name of the most famous track, artist, album or radio station that the search string probably refers to. The search string is often incomplete: a surname alone means the full name of the best-known artist with that name. Correct misspellings and use the official capitalization."
    },
    "category": {
      "type": "string",
      "enum": ["track", "artist", "album", "radio_station"],
      "description": "The most likely music entity type for the search string."
    },
    "confidence": {
      "type": "string",
      "enum": ["high", "medium", "low"]
    }
  }
}
```

`x-order` puts `full_title` first. The aim is that the model names the item before it selects the category. We did not test the other order.

### What made the full title better

Three changes were necessary:

1. **The field description.** It tells the model that the search string is often incomplete, and that a surname alone means the best-known artist with that name. Before this change, "sinatra" came back unchanged.
2. **The instructions.** "Identify what this music search string refers to." gives better titles than "Classify this music search string.". With the "classify" instructions, the model often copied the input ("rolling stones", "paranoid android"). The long instructions that list the categories made the titles worse ("dark side moon" came back unchanged).
3. **A label and quotes around the search string.** The prompt `Search string: "dylan"` is better than the prompt `dylan`. With this change, "Dylan" gives "Bob Dylan".

### Results

Results on 18 lowercase search strings, with all three changes:

| Search string | full_title | category | Result |
|---|---|---|---|
| sinatra | Frank Sinatra | artist | Correct |
| springsteen | Bruce Springsteen | artist | Correct |
| bowie | David Bowie | artist | Correct |
| coltrane | John Coltrane | artist | Correct |
| beatles | The Beatles | artist | Correct |
| rolling stones | The Rolling Stones | artist | Correct |
| taylor swift | Taylor Swift | artist | Correct |
| adele | Adele | artist | Correct |
| prince | Prince | artist | Correct |
| dark side moon | The Dark Side of the Moon | album | Correct |
| kind of blu | Kind of Blue | album | Correct |
| ok computer | OK Computer | album | Correct |
| paranoid andriod | Paranoid Android | track | Correct |
| wish you were here | Wish You Were Here | track | Correct |
| kexp | KEXP | radio_station | Correct |
| bbc radio 6 | BBC Radio 6 | radio_station | Title is not complete ("BBC Radio 6 Music") |
| dylan | Dylan | artist | Title is not complete ("Bob Dylan") |
| cash | Cash | track | Incorrect ("Johnny Cash", artist) |

Short search strings that are also usual words or first names ("dylan", "cash") are difficult for the model.

We also tried to capitalize each word of an all-lowercase search string before the call. This change gave "Bob Dylan" for "dylan", but "Rolling Stones" (without "The") for "rolling stones". We do not recommend it.

## Prefer album when a title is also a track

Tested on 2026-11-22: can the schema or the instructions make the model choose `album` for a title that is both an album and a track? Ten such titles: thriller, wish you were here, purple rain, hotel california, paranoid, heroes, nevermind, born in the usa, bad, 1999. The program's schema (Schema 2 without `confidence`) gave `track` for all ten, also for "nevermind", which is only an album.

| Change | Albums chosen | Side effects |
|---|---|---|
| `category` description adds: "When the search string is the title of both an album and a track, choose album." | 4 (Thriller, Wish You Were Here, Nevermind, 1999) | None seen |
| `category` description: "If the search string is the title of an album, choose album, even when a track has the same title. Choose track only for a title that is not also an album." | 2 | "paranoid andriod" became `artist`; "1999" became "99" |
| `album` first in the `enum` | 3 | None seen |
| Instructions add: "When a title is both an album and a track, it refers to the album." | 4 | "paranoid" became "Paranoid - Black Sabbath" |

Purple Rain, Hotel California, Paranoid, Heroes, Born in the USA and Bad stayed `track` in almost every test. The model possibly does not know that they are also album titles, so no wording can make it apply the rule. Make this preference in the program, with the search results, not in the model.

## Several candidates in one answer

Tested on 2026-11-22: can the model give more than one answer, so the program can choose? Two schemas, with the instructions "Identify what this music search string refers to.":

- **A, a ranked list.** `candidates`: an array of 1 to 3 objects with `full_title` and `category`, described as "What the search string probably refers to, the most likely first. Give a separate candidate when the same title is both an album and a track." Each object needs its own `title` and `x-order`, as the root does.
- **B, one field for each category.** `artist`, `album`, `track` and `radio_station`, each "the best-known one the search string probably refers to, or an empty string when none fits".

Results:

- **A** gave an `album` candidate for 9 of the 10 titles in the section above; only "heroes" gave a track and an artist. The first candidate agreed with the single answer for almost every search; "paranoid andriod" put `album` first and `track` second. Some titles were noisy: "Bruce Springsteen - Born in the USA" as a track, "The Dark Side of the Moon (track)". "dylan" and "cash" stayed weak.
- **B** filled every field, also when nothing fits: "paranoid" gave the album "The Dark Side of the Moon" and the track "The Wall", "1999" gave "Billie Jean", and every search got a radio station. Do not use it.
- **Time.** A took 0.7 to 1.3 s, against about 0.55 s for one answer, because the answer is longer.

A is slower than one answer (see "Speed" below). The program's classifier does not use it.

## Speed: what the model writes counts, not what it reads

Measured on 2026-11-22, warm model, 18 search strings each (the 10 album-and-track titles and 8 others), median and maximum time:

| Schema | Median | Max | Notes |
|---|---|---|---|
| One answer (`full_title`, `category`) | 0.53 s | 0.59 s | Album for none of the 10 titles |
| One `full_title` and a ranked `categories` array (1 to 3, "Give both album and track when the title is both") | 0.57 s | 0.62 s | Album for 6 of 10; "1999" became "99 Luftballons" |
| Candidate list (A above) | 0.91 s | 1.44 s | Album for 9 of 10; "kind of blu" gave "Kendrick Lamar -" three times |
| Candidate list, short `full_title` description | 0.88 s | 1.38 s | "sinatra" stayed "Sinatra" |
| Candidate list, minimal descriptions | 0.84 s | 1.18 s | "sinatra" gave "The Voice of Sinatra" |
| Candidate list, at most 2 | 0.90 s | 1.16 s | |
| Candidate list, keys `title` and `type` | 0.85 s | 1.28 s | |
| Candidate list, all of the above | 0.81 s | 0.99 s | "ozzy" stayed "Ozzy" |

- Shorter instructions and descriptions save about 0.1 s and cost quality: the long `full_title` description is what turns a surname into a full name.
- The time follows the length of the answer. A ranked `categories` array after one `full_title` adds almost nothing to one answer.
- The first call after a pause takes up to 1.8 s, while the model loads.

The program's classifier (`internal/localmodel`) uses one `full_title` and a ranked `categories` array.

## Give the schema without a file

`--schema` also accepts the JSON text. The help text does not say this, but `fm` tries the argument as JSON and as a file path.

### Inline schema on the command line

Write the full schema in the argument. Use single quotes, thus the shell does not change the double quotes in the JSON:

```bash
fm respond --no-stream -g \
  -i 'Identify what this music search string refers to.' \
  --schema '{"title":"MusicClassification","type":"object","additionalProperties":false,"x-order":["full_title","category","confidence"],"required":["full_title","category","confidence"],"properties":{"full_title":{"type":"string","description":"The full official name of the most famous track, artist, album or radio station that the search string probably refers to. The search string is often incomplete: a surname alone means the full name of the best-known artist with that name. Correct misspellings and use the official capitalization."},"category":{"type":"string","enum":["track","artist","album","radio_station"],"description":"The most likely music entity type for the search string."},"confidence":{"type":"string","enum":["high","medium","low"]}}}' \
  'Search string: "bowie"'
# {"category": "artist", "full_title": "David Bowie", "confidence": "high"}
```

The schema can also come from a variable or a file:

```bash
SCHEMA=$(cat enum.json)
fm respond --no-stream -g -i 'Identify what this music search string refers to.' \
  --schema "$SCHEMA" 'Search string: "bowie"'
```

The prompt can come from stdin when the schema is an argument:

```bash
echo 'Search string: "coltrane"' | fm respond --no-stream -g \
  -i 'Identify what this music search string refers to.' --schema "$SCHEMA"
# {"confidence": "high", "full_title": "John Coltrane", "category": "artist"}
```

An inline schema must also have `title` and `x-order`. If it does not, the error is `Invalid schema at '<inline JSON>'`.

### Methods that did not work

| Method | Result |
|---|---|
| `--schema -` | Fails. `fm` looks for a file with the name "-". |
| `--schema /dev/stdin` | Fails with "you don't have permission to view it". |
| `--schema <(...)` | Fails with "you don't have permission to view it". |

The last two failures possibly come from the sandbox that we used for the tests. Try them again in a usual terminal if you need them.

## fm serve compared with fm respond

Measured on 2026-11-22 with Schema 2, the same instructions and the same prompts. `fm serve --socket <path>` was called with `POST /v1/chat/completions`: the instructions as the `system` message, the prompt as the `user` message, `"temperature": 0`, `"stream": false` and the schema in `response_format` (`json_schema`). Without `"stream": false` it streams.

| Search string | fm respond | fm serve |
|---|---|---|
| bowie (first call) | 0.64 s, David Bowie, artist | 1.26 s, "David Bowie - The Man Who Fell to Earth", album |
| dark side moon | 0.67 s, The Dark Side of the Moon, album | 0.59 s, "Dark Side of the Moon", album |
| kexp | 0.59 s, KEXP, radio_station | 0.56 s, KEXP, artist |
| springsteen | 0.60 s, Bruce Springsteen, artist | 0.62 s, "Born to Run", album |
| kind of blu | 0.58 s, Kind of Blue, album | 0.55 s, Kind of Blue, album |

- **Speed.** Once the model is loaded, both take about 0.6 s. Starting a new `fm` process costs almost nothing; the model's work is the time. The first call after a pause is slower in both (up to 1.8 s seen with `fm respond`).
- **Answers.** `fm serve` was wrong on 3 of 5. The server possibly does not give the `system` message as instructions, or does not use the schema's descriptions and `x-order` the same way. We did not look further.
- **Conclusion.** Use `fm respond`. It needs no server to start, keep running or stop.

## Use from another program

- **Output.** The result is one line of JSON on stdout. Errors go to stderr, and the exit code is 1.
- **Key order.** The order of the keys in the output changes from call to call. Read the JSON by key name, not by position.
- **Arguments.** Start `fm` directly with a list of arguments, not through a shell. Then the schema JSON does not need quotes or escape characters.
- **Same input, same output.** Use `-g`.
- **Confidence.** The model gave `high` for all tests, also for incorrect results. The program's classifier (`internal/localmodel`) has no `confidence`; it uses one `full_title` and a ranked `categories` array (see "Speed").
- **Full title.** The full title is a guess, and the model always gives one. If a search for `full_title` finds nothing, also search for the original search string.
- **Inline schema.** The JSON text in `--schema` is not in the help text. A future version of `fm` can remove it. If it stops working, write the schema to a temporary file.
