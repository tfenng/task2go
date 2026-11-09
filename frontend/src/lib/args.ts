export function parseShellArgs(input: string): string[] {
  const result: string[] = [];
  let current = "";
  let quote: '"' | "'" | null = null;
  for (let i = 0; i < input.length; i += 1) {
    const char = input[i];
    if (quote) {
      if (char === quote) {
        quote = null;
      } else {
        current += char;
      }
    } else if (char === '"' || char === "'") {
      quote = char;
    } else if (/\s/.test(char)) {
      if (current) {
        result.push(current);
        current = "";
      }
    } else {
      current += char;
    }
  }
  if (current) result.push(current);
  return result;
}

export function scriptBaseName(argv: string): string {
  const lines = argv.split("\n").map((line) => line.trim()).filter(Boolean);
  const candidate = lines.find((line) => /\.(py|sh|bash|js|rb|pl|exe)$/i.test(line)) || lines[1] || lines[0] || "";
  const name = candidate.split(/[\\/]/).pop() || candidate;
  return name.replace(/\.[^.]+$/, "") || "task";
}
