import sys, re
p = 'web/src/services/api.ts'
with open(p, 'r', encoding='utf-8') as f:
    c = f.read()

# Remove the upload debug-logging block inside the response interceptor.
pattern = re.compile(
    r"    // Log all upload responses for debugging\n"
    r"    if \(response\.config\.url\?\.includes\('/upload'\)\) \{\n"
    r"(?:      console\.log\('\[API Response\] [^']*':.*\);\n)+"
    r"    \}\n"
)
new, n = pattern.subn("", c)
if n == 0:
    print("pattern not matched", file=sys.stderr); sys.exit(1)
with open(p, 'w', encoding='utf-8') as f:
    f.write(new)
print(f"OK ({n} removed)")
