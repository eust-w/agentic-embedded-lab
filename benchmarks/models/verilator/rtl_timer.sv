module rtl_timer #(parameter bit FAULTY = 1'b0);
  integer count = 0;
  integer failure = 0;
  initial begin
    repeat (8) begin #1; count = count + 1; end
    failure = FAULTY ? (count == 8) : (count != 8);
    $display("AEL_METRIC count=%0d", count);
    $display("AEL_METRIC failure=%0d", failure);
    $display("AEL_EVENT verilator.timer {\"cycles\":8,\"faulty\":%0d}", FAULTY);
    $finish;
  end
endmodule
