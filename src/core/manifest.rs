use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::path::{Path, PathBuf};
use serde_json::Value;
use anyhow::{Context, Result};
use std::fs;

#[derive(Debug, Serialize, Deserialize, Clone, Default)]
pub struct XfpmManifest {
    #[serde(default = "default_version")]
    pub version: String,
    
    #[serde(default)]
    pub globals: HashMap<String, String>,
    
    #[serde(default)]
    pub catalogs: HashMap<String, HashMap<String, String>>,
    
    #[serde(default)]
    pub packages: Vec<String>,
    
    #[serde(default)]
    pub settings: HashMap<String, Value>,
    
    #[serde(default)]
    pub hooks: HashMap<String, String>,
}

fn default_version() -> String {
    "1.0".to_string()
}

#[derive(Debug, Serialize, Deserialize, Clone, Default)]
pub struct XfpmPackageConfig {
    #[serde(default)]
    pub extends: Option<String>,
    
    #[serde(default)]
    pub optimize: Vec<String>,
    
    #[serde(default)]
    pub vars: HashMap<String, String>,
}

impl XfpmManifest {
    pub fn load_from_dir(dir: &Path) -> Result<Option<Self>> {
        let manifest_path = dir.join("xfpm.yaml");
        if manifest_path.exists() {
            let content = fs::read_to_string(&manifest_path)
                .with_context(|| format!("Failed to read {:?}", manifest_path))?;
            let manifest: XfpmManifest = serde_yaml::from_str(&content)
                .with_context(|| format!("Failed to parse {:?}", manifest_path))?;
            return Ok(Some(manifest));
        }
        
        // Fallback or recursive search upwards
        if let Some(parent) = dir.parent() {
            return Self::load_from_dir(parent);
        }
        
        Ok(None)
    }

    #[allow(dead_code)]
    pub fn find_root(start_path: &Path) -> Option<PathBuf> {
        let mut curr = start_path.to_path_buf();
        loop {
            if curr.join("xfpm.yaml").exists() || curr.join("pnpm-workspace.yaml").exists() {
                return Some(curr);
            }
            if !curr.pop() {
                break;
            }
        }
        None
    }
}
