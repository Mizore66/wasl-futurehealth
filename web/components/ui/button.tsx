import * as React from "react"
import { cva, type VariantProps } from "class-variance-authority"
import { cn } from "@/lib/utils"
import { Slot } from "radix-ui"

/**
 * Pressed feedback every variant shares: the button squeezes to 0.96 over 150ms and springs back on release.
 * Transitions paint-only properties (colour, shadow, transform/scale), never a layout property. No squeeze under reduced motion, and none on a
 * disabled, aria-disabled or aria-busy button, so a control that will not act does not look as if it did.
 */
const press =
  "transition-[color,background-color,border-color,box-shadow,transform,scale] duration-150 ease-out motion-safe:not-disabled:not-aria-disabled:not-aria-busy:active:scale-[0.96]"

/** The stock shadcn look: a 3px ring replaces the app's global focus outline. */
const stock =
  "shrink-0 rounded-md text-sm font-medium whitespace-nowrap outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50"

const buttonVariants = cva(
  `inline-flex items-center justify-center gap-2 ${press} disabled:pointer-events-none disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4`,
  {
    variants: {
      variant: {
        default: `${stock} bg-primary text-primary-foreground hover:bg-primary/90`,
        destructive: `${stock} bg-destructive text-white hover:bg-destructive/90 focus-visible:ring-destructive/20 dark:bg-destructive/60 dark:focus-visible:ring-destructive/40`,
        outline: `${stock} border bg-background shadow-xs hover:bg-accent hover:text-accent-foreground dark:border-input dark:bg-input/30 dark:hover:bg-input/50`,
        secondary: `${stock} bg-secondary text-secondary-foreground hover:bg-secondary/80`,
        ghost: `${stock} hover:bg-accent hover:text-accent-foreground dark:hover:bg-accent/50`,
        link: `${stock} text-primary underline-offset-4 hover:underline`,
        // Product actions on the ledger tokens. They keep the app's global :focus-visible outline.
        /** Primary action: the one thing to do next. */
        cta: "cursor-pointer rounded-control bg-cta text-body font-medium text-white",
        /** Secondary action beside a primary one. */
        quiet: "cursor-pointer rounded-control bg-active text-body text-ink hover:bg-control",
        /** A one-tap choice among several, such as a call outcome. The caller sets the border colour. */
        chip: "cursor-pointer rounded-chip border bg-surface text-body text-ink hover:bg-ground",
      },
      size: {
        default: "h-9 px-4 py-2 has-[>svg]:px-3",
        xs: "h-6 gap-1 rounded-md px-2 text-xs has-[>svg]:px-1.5 [&_svg:not([class*='size-'])]:size-3",
        sm: "h-8 gap-1.5 rounded-md px-3 has-[>svg]:px-2.5",
        lg: "h-10 rounded-md px-6 has-[>svg]:px-4",
        /** The product's 44px touch target. */
        touch: "h-11 px-4",
        icon: "size-9",
        "icon-xs": "size-6 rounded-md [&_svg:not([class*='size-'])]:size-3",
        "icon-sm": "size-8",
        "icon-lg": "size-10",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  }
)

function Button({
  className,
  variant = "default",
  size = "default",
  asChild = false,
  ...props
}: React.ComponentProps<"button"> &
  VariantProps<typeof buttonVariants> & {
    asChild?: boolean
  }) {
  const Comp = asChild ? Slot.Root : "button"

  return (
    <Comp
      data-slot="button"
      data-variant={variant}
      data-size={size}
      className={cn(buttonVariants({ variant, size, className }))}
      {...props}
    />
  )
}

export { Button, buttonVariants }
