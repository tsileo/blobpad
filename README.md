BlobPad
=======

BlobPad is a note taking application build on top of [BlobStash](https://github.com/tsileo/blobstash).

## Features

- Markdown only notes
- Full notes history
- Possibility to encrypt notes using [NaCl secretbox](http://nacl.cr.yp.to/secretbox.html) (via BlobStash)
- No delete feature by design, deleted notes/notebooks stay in a special trash

## Based upon

- [BlobStash](https://github.com/tsileo/blobstash), for blob store/database
- [UIkit](http://getuikit.com/), [Ractive.js](http://www.ractivejs.org/), [CodeMirror](http://codemirror.net/), for the UI
- [Elasticsearch](http://www.elasticsearch.org/), for the search

## Roadmap

- PDF support with pdf.js (+ OCR)
