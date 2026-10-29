import { cva } from 'class-variance-authority';

// Native controls and React projections share these visual definitions.
export const buttonVariants = cva(
  'inline-flex shrink-0 items-center justify-center gap-2 rounded-md text-sm font-medium transition-colors outline-none disabled:pointer-events-none disabled:opacity-45 [&_svg]:pointer-events-none [&_svg]:size-5 [&_svg]:stroke-2',
  {
    variants: {
      variant: {
        default: 'bg-foreground text-background hover:bg-foreground/86',
        secondary: 'border border-border bg-background hover:bg-muted',
      },
      size: {default: 'h-9 px-4 py-2', icon: 'size-9'},
    },
    defaultVariants: {variant: 'default', size: 'default'},
  },
);

export const textareaStyle = 'min-h-20 w-full resize-none rounded-md border-0 bg-transparent px-3 py-2 text-sm leading-relaxed outline-none placeholder:text-muted-foreground';
