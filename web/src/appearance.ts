try{const saved=localStorage.getItem('steward-theme');document.documentElement.classList.toggle('dark',saved==='dark'||!saved&&matchMedia('(prefers-color-scheme: dark)').matches)}catch{}
