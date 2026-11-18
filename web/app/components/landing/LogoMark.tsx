export default function LogoMark({ className = "h-7 w-7" }: { className?: string }) {
  return (
    <svg viewBox="0 0 32 32" className={className} aria-hidden="true">
      <path d="M16 3 28 10v12l-12 7-12-7V10z" fill="#2B3AE8" />
      <path d="M16 3 28 10l-12 7-12-7z" fill="#7C87F2" />
      <path d="M16 17v12l12-7V10z" fill="#1E2BC4" />
    </svg>
  );
}
