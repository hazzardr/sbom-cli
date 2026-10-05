# Test fixtures

Every fixture is a published example SBOM, copied unmodified. Do not add
hand-written SBOMs here; tests should exercise real documents.

| File | Source | License |
|---|---|---|
| `spdx-3.0.1-spec-package-sbom.json` | [spdx/spdx-spec `examples/jsonld/package_sbom.json`](https://github.com/spdx/spdx-spec/blob/c237baaf68ec4c10fbd6a0b403b520060d11de79/examples/jsonld/package_sbom.json) | Community-Spec-1.0 |
| `cyclonedx-1.7-guide-bom-link.json` | [OWASP CycloneDX Authoritative Guide to SBOM](https://cyclonedx.org/guides/OWASP_CycloneDX-Authoritative-Guide-to-SBOM-en.pdf), Third Edition (2025-10-21), p. 61, "Linking to Objects Within The Same BOM". Transcribed from the PDF text; trailing whitespace removed. | CC-BY-4.0, © The OWASP Foundation |
| `spdx-3.0.1-examples-example11.json` | [spdx/spdx-examples `software/example11/spdx3.0/sbom.spdx3.json`](https://github.com/spdx/spdx-examples/blob/af7e2804a115b716335b4ead887e67fe9018fd60/software/example11/spdx3.0/sbom.spdx3.json) | CC0-1.0 (SPDX documents in that repo) |
| `cyclonedx-1.6-spec-valid-bom.json` | [CycloneDX/specification `tools/src/test/resources/1.6/valid-bom-1.6.json`](https://github.com/CycloneDX/specification/blob/1ce97b2a7b8cf2429da248560d2aa671c6bce74a/tools/src/test/resources/1.6/valid-bom-1.6.json) | Apache-2.0 |
| `cyclonedx-1.7-spec-license-choice.json` | [CycloneDX/specification `tools/src/test/resources/1.7/valid-license-choice-1.7.json`](https://github.com/CycloneDX/specification/blob/1ce97b2a7b8cf2429da248560d2aa671c6bce74a/tools/src/test/resources/1.7/valid-license-choice-1.7.json) | Apache-2.0 |

The first two were chosen by the project owner. They contain no licenses or
package URLs, so the last three were added to cover license parsing and
`query --license` against real documents.

## `unsupported/`

Real documents in formats the CLI rejects, used to test that rejection.

| File | Source | License |
|---|---|---|
| `cyclonedx-1.4-examples-laravel.json` | [CycloneDX/bom-examples `SBOM/laravel-7.12.0/bom.1.4.json`](https://github.com/CycloneDX/bom-examples/blob/f8af8ae21c726318e3085d49ebb8f122b3dbe5e9/SBOM/laravel-7.12.0/bom.1.4.json) | CC0-1.0 |
| `spdx-2.3-examples-minimal-sbom.json` | [spdx/spdx-examples `presentations/OSS-NA-2023/SPDXVersion2.3/01-MinimalSBOM.json`](https://github.com/spdx/spdx-examples/blob/31e90a206f61ca9970a184cd7790d5af9ad92e6c/presentations/OSS-NA-2023/SPDXVersion2.3/01-MinimalSBOM.json) | CC0-1.0 (SPDX documents in that repo) |

No SPDX 3.1 document has been published yet, so that rejection case is an
inline string in `sbom_test.go`.
