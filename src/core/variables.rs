use std::collections::HashMap;

pub struct VariableResolver {
    context: HashMap<String, String>,
}

impl VariableResolver {
    pub fn new() -> Self {
        Self {
            context: HashMap::new(),
        }
    }

    pub fn with_context(context: HashMap<String, String>) -> Self {
        Self { context }
    }

    #[allow(dead_code)]
    pub fn add_var(&mut self, name: &str, value: &str) {
        self.context.insert(name.to_string(), value.to_string());
    }

    pub fn resolve(&self, input: &str) -> String {
        self.resolve_recursive(input, 0)
    }

    fn resolve_recursive(&self, input: &str, depth: usize) -> String {
        if depth > 10 {
            return input.to_string(); // Prevent infinite recursion
        }

        let mut output = input.to_string();
        let mut changed = false;

        for (name, value) in &self.context {
            let var_pattern = format!("${}", name);
            if output.contains(&var_pattern) {
                output = output.replace(&var_pattern, value);
                changed = true;
            }
        }

        if changed {
            self.resolve_recursive(&output, depth + 1)
        } else {
            output
        }
    }

    /// Extends current context with env variables
    pub fn inject_env(&mut self) {
        for (key, value) in std::env::vars() {
            self.context.insert(format!("ENV_{}", key), value);
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_variable_resolution() {
        let mut resolver = VariableResolver::new();
        resolver.add_var("major", "19");
        resolver.add_var("version", "$major.0.1");
        
        assert_eq!(resolver.resolve("react@$version"), "react@19.0.1");
    }
}
