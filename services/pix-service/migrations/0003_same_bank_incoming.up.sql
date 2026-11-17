-- A same-bank Pix is recorded twice: the sender's outgoing row and the
-- receiver's incoming row (so the receiver can refund it). Both share the
-- end-to-end id, which is therefore unique per direction.
ALTER TABLE pix_transfers DROP CONSTRAINT pix_transfers_end_to_end_id_key;
ALTER TABLE pix_transfers ADD CONSTRAINT pix_transfers_e2e_direction_key UNIQUE (end_to_end_id, direction);
