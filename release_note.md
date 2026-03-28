# Release Note - XFPM G0.1.59

## [G0.1.59] - 2026-03-28

### Fixed

- **Core**: Fixed a critical bug where `xfpm install <pkg>` (or `xfpm i <pkg>`) would fail to create a `package.json` file in a directory that did not already have one. XFPM now correctly initializes a default `package.json` with the current directory name and version `1.0.0` when installing packages for the first time.

### Improved

- **DX**: Streamlined first-time initialization experience, making it easier to start new projects by directly adding dependencies.
