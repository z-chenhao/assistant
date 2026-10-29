import { cva } from 'class-variance-authority';

// Native controls and React projections share these visual definitions.
export const buttonVariants = cva(
  'ui-button',
  {
    variants: {
      variant: {
        default: '',
        secondary: 'secondary',
      },
      size: {default: '', icon: 'icon-button'},
    },
    defaultVariants: {variant: 'default', size: 'default'},
  },
);

export const textareaStyle = 'min-h-20 w-full resize-none rounded-md border-0 bg-transparent px-3 py-2 text-sm leading-relaxed outline-none placeholder:text-muted-foreground';
