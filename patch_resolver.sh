sed -i 's/utils.Log("SKIP".*/f, _ := os.OpenFile("\/tmp\/xfpm.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); f.WriteString("SKIP " + realName + "\\n"); f.Close()/g' internal/core/resolver.go
sed -i 's/utils.Log("DEBUG".*/f, _ := os.OpenFile("\/tmp\/xfpm.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); f.WriteString("DEBUG " + realName + "\\n"); f.Close()/g' internal/core/resolver.go
