<!--
/***************************************************************************
 * XFPM (XyPriss Fast Package Manager)
 *
 * @author Nehonix
 * @license Nehonix OSL (NOSL)
 *
 * Copyright (c) 2025 Nehonix. All rights reserved.
 *
 * This License governs the use, modification, and distribution of software
 * provided by NEHONIX under its open source projects.
 * NEHONIX is committed to fostering collaborative innovation while strictly
 * protecting its intellectual property rights.
 ***************************************************************************/
-->

# Architecture: Storage Optimization & Global CAS

## Overview

To ensure maximum performance and minimal disk footprint, XFPM-GO (G0.1.68+) introduces a centralized **Content-Addressable Storage (CAS)** system. This architecture allows multiple projects to share the same physical files on disk, effectively reducing the space required for redundant dependencies to near-zero.

## Core Components

### 1. Global Store (`~/.xpm/storage`)

The storage is moved out of the project-local `node_modules` and into a global user directory.

- `files/`: Contains the actual content-addressed data blobs, indexed by their BLAKE3 hashes.
- `indices/`: Stores JSON maps for each `package@version`, mapping internal relative paths to their corresponding hashes.
- `temp/`: Used for atomic writes and streaming extractions.

### 2. Tiered Linking Mechanism

XFPM-GO uses a prioritized linking strategy to populate `node_modules` from the global store:

| Priority | Method            | Description                                                                       | Platform          |
| :------- | :---------------- | :-------------------------------------------------------------------------------- | :---------------- |
| **1**    | **Reflink (CoW)** | Uses `ioctl(FICLONE)` to create Copy-on-Write clones. Zero space, full isolation. | Linux (XFS/Btrfs) |
| **2**    | **Hardlink**      | Shares the same inode. Zero space, uses Read-Only bits for safety.                | Cross-platform    |
| **3**    | **Copy**          | Standard data copy. Used as a last resort fallback.                               | Cross-platform    |

### 3. Virtual Store Layout

XFPM follows a strict isolation pattern:

1.  Packages are "extracted" (linked) into `.xpm/virtual_store/pkg@version/node_modules/pkg`.
2.  Dependencies are symlinked into the virtual store's internal `node_modules`.
3.  The project's root `node_modules/pkg` is a symlink to the virtual store.

## Integrity & Security

### Read-Only Invariant

All files stored in the CAS are marked as **Read-Only** (`0444` or `0555`). This is a critical security measure to prevent post-install scripts or accidental edits in one project from corrupting the shared dependency for all other projects.

### Atomic Operations

- Downloads are streamed through a hasher and written to a temporary file.
- The file is only moved to its final destination in the CAS once the hash is verified and permissions are locked.

## Environment Overrides

- `XFPM_STORAGE`: Allows users to specify a custom global storage path (e.g., for different partitions).
