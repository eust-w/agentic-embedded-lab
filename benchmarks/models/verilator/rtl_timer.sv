module rtl_timer #(parameter bit FAULTY = 1'b0);
  integer count = 0;
  integer wraps = 0;
  integer irq_count = 0;
  integer hold_violations = 0;
  integer failure = 0;
  integer held_count = 0;

  task automatic enabled_tick;
    begin
      #1;
      if (FAULTY) begin
        count = count + 1;
      end else begin
        if (count == 15) begin
          count = 0;
          wraps = wraps + 1;
        end else begin
          count = count + 1;
        end
      end
      if (count == 3) irq_count = irq_count + 1;
    end
  endtask

  initial begin
    // Reset, enable, compare/IRQ, disabled hold, wrap and re-arm sequence.
    count = 0;
    repeat (3) enabled_tick();
    held_count = count;
    #1;
    if (count != held_count) hold_violations = hold_violations + 1;
    repeat (20) enabled_tick();
    failure = (count != 7) || (wraps != 1) || (irq_count != 2) || (hold_violations != 0);
    $display("AEL_METRIC count=%0d", count);
    $display("AEL_METRIC wraps=%0d", wraps);
    $display("AEL_METRIC irq_count=%0d", irq_count);
    $display("AEL_METRIC hold_violations=%0d", hold_violations);
    $display("AEL_METRIC failure=%0d", failure);
    $display("AEL_EVENT verilator.timer {\"enabled_cycles\":23,\"counter_width\":4,\"compare\":3,\"faulty\":%0d}", FAULTY);
    $finish;
  end
endmodule
