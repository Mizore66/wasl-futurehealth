"use client";

import { Dialog } from "radix-ui";

/**
 * A modal sheet from the bottom of the phone column. Rendered in place, not in a portal,
 * so it inherits the phone's `lang`, `dir` and font.
 */
export function BottomSheet({
  trigger,
  title,
  closeLabel,
  open,
  onOpenChange,
  children,
}: {
  trigger: React.ReactElement;
  title: string;
  closeLabel: string;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  children: React.ReactNode;
}) {
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Trigger asChild>{trigger}</Dialog.Trigger>
      <Dialog.Overlay className="fixed inset-0 z-40 bg-[rgba(10,10,10,0.32)] data-[state=open]:animate-in data-[state=open]:fade-in-0 motion-reduce:animate-none" />
      <Dialog.Content
        aria-describedby={undefined}
        className="fixed inset-x-0 bottom-0 z-50 mx-auto w-full max-w-[390px] rounded-t-card bg-surface px-5 pt-6 pb-7 data-[state=open]:animate-in data-[state=open]:slide-in-from-bottom-8 motion-reduce:animate-none"
      >
        <Dialog.Title className="m-0 mb-4 text-[20px] font-semibold">{title}</Dialog.Title>
        {children}
        <Dialog.Close className="mt-5 h-[52px] w-full cursor-pointer rounded-control border-0 bg-active text-[16px] font-medium text-ink">
          {closeLabel}
        </Dialog.Close>
      </Dialog.Content>
    </Dialog.Root>
  );
}
